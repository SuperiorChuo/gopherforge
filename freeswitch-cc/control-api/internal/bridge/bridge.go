// Package bridge translates raw FreeSWITCH channel events into the
// integration contract's call.* webhook events (FS-M2), accumulating per-call
// state so both the local CDR and the pushed payload stay complete.
package bridge

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/webhook"
)

// Events is the ESL subscription list Bridge understands.
var Events = []string{"CHANNEL_CREATE", "CHANNEL_PROGRESS_MEDIA", "CHANNEL_ANSWER", "CHANNEL_HANGUP_COMPLETE", "RECORD_STOP"}

type callState struct {
	direction  string
	caller     string
	callee     string
	agentExt   string
	recording  string
	tenantID   uint64
	voiceID    uint64
	startedAt  time.Time
	answeredAt time.Time
}

// CallStore is the CDR sink (satisfied by *store.Store; narrowed for tests).
type CallStore interface {
	UpsertCall(*store.Call) error
}

type Bridge struct {
	Store CallStore
	Hook  *webhook.Client

	mu    sync.Mutex
	calls map[string]*callState // key: A-leg Unique-ID
}

func New(st CallStore, hook *webhook.Client) *Bridge {
	return &Bridge{Store: st, Hook: hook, calls: make(map[string]*callState)}
}

// Handle consumes one parsed ESL event. Only the A-leg is tracked
// (Unique-ID == Channel-Call-UUID); B-legs of bridged calls are skipped so an
// internal extension call yields one CDR row, not two.
func (b *Bridge) Handle(ev map[string]string) {
	// 早媒体分类样本是瞬时技术文件，不得冒充正式通话录音回写/转写。
	if ev["Event-Name"] == "RECORD_STOP" && strings.Contains(ev["Record-File-Path"], "/early-media/") {
		return
	}
	uuid := ev["Unique-ID"]
	if uuid == "" || uuid != ev["Channel-Call-UUID"] {
		return
	}

	// 租户标记随任一事件出现即记录（export 发生在拨号计划执行期，CHANNEL_CREATE
	// 时通常还没有；后续事件带出后补齐，webhook 载荷从此含 tenant_id）。
	if st := b.get(uuid, ev); st != nil {
		b.mu.Lock()
		if id, err := strconv.ParseUint(ev["variable_cc_tenant_id"], 10, 64); err == nil && id > 0 {
			st.tenantID = id
		}
		if id, err := strconv.ParseUint(ev["variable_cc_voice_id"], 10, 64); err == nil && id > 0 {
			st.voiceID = id
		}
		b.mu.Unlock()
	}

	switch ev["Event-Name"] {
	case "CHANNEL_CREATE":
		st := b.get(uuid, ev)
		b.mu.Lock()
		st.direction = direction(ev)
		st.caller = firstOf(ev, "Caller-Caller-ID-Number", "Caller-ANI")
		st.callee = ev["Caller-Destination-Number"]
		st.agentExt = ev["Caller-Destination-Number"]
		if st.startedAt.IsZero() {
			st.startedAt = time.Now()
		}
		b.mu.Unlock()
		b.emit("call.ringing", uuid, st, nil)

	case "CHANNEL_ANSWER":
		st := b.get(uuid, ev)
		b.mu.Lock()
		st.answeredAt = time.Now()
		b.mu.Unlock()
		b.emit("call.answered", uuid, st, nil)

	case "RECORD_STOP":
		st := b.get(uuid, ev)
		b.mu.Lock()
		st.recording = ev["Record-File-Path"]
		b.mu.Unlock()
		b.emit("recording.ready", uuid, st, nil)

	case "CHANNEL_HANGUP_COMPLETE":
		st := b.get(uuid, ev)
		extra := map[string]any{"cause": ev["Hangup-Cause"]}
		if sec, err := strconv.Atoi(ev["variable_billsec"]); err == nil {
			extra["duration_sec"] = sec
		}
		b.emit("call.hangup", uuid, st, extra)
		b.mu.Lock()
		delete(b.calls, uuid)
		b.mu.Unlock()
	}
}

// get returns tracked state, backfilling from the event itself when the
// listener (re)connected mid-call and missed CHANNEL_CREATE.
func (b *Bridge) get(uuid string, ev map[string]string) *callState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if st, ok := b.calls[uuid]; ok {
		return st
	}
	st := &callState{
		direction: direction(ev),
		caller:    firstOf(ev, "Caller-Caller-ID-Number", "Caller-ANI"),
		callee:    ev["Caller-Destination-Number"],
		agentExt:  ev["Caller-Destination-Number"],
	}
	b.calls[uuid] = st
	return st
}

func (b *Bridge) emit(event, uuid string, st *callState, extra map[string]any) {
	b.mu.Lock()
	payload := map[string]any{
		"call_id":   uuid,
		"direction": st.direction,
		"caller":    st.caller,
		"callee":    st.callee,
		"agent_ext": st.agentExt,
	}
	if st.recording != "" {
		payload["recording"] = st.recording
	}
	if st.tenantID > 0 {
		payload["tenant_id"] = st.tenantID
	}
	if st.voiceID > 0 {
		payload["voice_id"] = st.voiceID
	}
	rec := &store.Call{
		CallID:    uuid,
		Direction: st.direction,
		Caller:    st.caller,
		Callee:    st.callee,
		AgentExt:  st.agentExt,
		Recording: st.recording,
	}
	if !st.startedAt.IsZero() {
		t := st.startedAt
		rec.StartedAt = &t
	}
	if !st.answeredAt.IsZero() {
		t := st.answeredAt
		rec.AnsweredAt = &t
	}
	b.mu.Unlock()

	switch event {
	case "call.ringing":
		rec.Status = "ringing"
	case "call.answered":
		rec.Status = "answered"
	case "recording.ready":
		rec.Status = "recording_ready"
	case "call.hangup":
		rec.Status = "hangup"
		now := time.Now()
		rec.EndedAt = &now
	}
	for k, v := range extra {
		payload[k] = v
	}
	if d, ok := extra["duration_sec"].(int); ok {
		rec.DurationSec = d
	}

	if err := b.Store.UpsertCall(rec); err != nil {
		log.Printf("bridge: cdr upsert %s %s: %v", event, uuid, err)
	}
	if b.Hook.Enabled() {
		if err := b.Hook.Send(event, payload); err != nil {
			log.Printf("bridge: webhook %s %s: %v", event, uuid, err)
		}
	}
}

func direction(ev map[string]string) string {
	if d := ev["Call-Direction"]; d != "" {
		return d
	}
	return ev["Caller-Direction"]
}

func firstOf(ev map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := ev[k]; v != "" {
			return v
		}
	}
	return ""
}
