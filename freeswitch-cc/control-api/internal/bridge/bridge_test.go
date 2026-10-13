package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/webhook"
)

type fakeStore struct {
	calls []store.Call
}

func (f *fakeStore) UpsertCall(c *store.Call) error {
	f.calls = append(f.calls, *c)
	return nil
}

func aLeg(name string, extra map[string]string) map[string]string {
	ev := map[string]string{
		"Event-Name":                name,
		"Unique-ID":                 "uuid-a",
		"Channel-Call-UUID":         "uuid-a",
		"Call-Direction":            "inbound",
		"Caller-Caller-ID-Number":   "1000",
		"Caller-Destination-Number": "1001",
	}
	for k, v := range extra {
		ev[k] = v
	}
	return ev
}

func TestHandleLifecycle(t *testing.T) {
	fs := &fakeStore{}
	b := New(fs, webhook.New("", "")) // hook disabled

	b.Handle(aLeg("CHANNEL_CREATE", nil))
	b.Handle(aLeg("CHANNEL_ANSWER", nil))
	b.Handle(aLeg("CHANNEL_HANGUP_COMPLETE", map[string]string{
		"Hangup-Cause":     "NORMAL_CLEARING",
		"variable_billsec": "42",
	}))

	if len(fs.calls) != 3 {
		t.Fatalf("want 3 upserts, got %d", len(fs.calls))
	}
	if fs.calls[0].Status != "ringing" || fs.calls[0].StartedAt == nil {
		t.Errorf("ringing: %+v", fs.calls[0])
	}
	if fs.calls[1].Status != "answered" || fs.calls[1].AnsweredAt == nil || fs.calls[1].StartedAt == nil {
		t.Errorf("answered should keep started_at: %+v", fs.calls[1])
	}
	last := fs.calls[2]
	if last.Status != "hangup" || last.DurationSec != 42 || last.EndedAt == nil {
		t.Errorf("hangup: %+v", last)
	}
	if last.Caller != "1000" || last.Callee != "1001" || last.CallID != "uuid-a" {
		t.Errorf("identity fields: %+v", last)
	}

	// state cleaned up after hangup
	b.mu.Lock()
	n := len(b.calls)
	b.mu.Unlock()
	if n != 0 {
		t.Errorf("call state not cleaned: %d", n)
	}
}

func TestHandleSkipsEarlyMediaRecordingStop(t *testing.T) {
	fs := &fakeStore{}
	b := New(fs, webhook.New("", ""))
	b.Handle(aLeg("RECORD_STOP", map[string]string{
		"Record-File-Path": "/var/lib/freeswitch/early-media/early-uuid-a.wav",
	}))
	if len(fs.calls) != 0 {
		t.Fatalf("early-media sample must not become recording.ready: %+v", fs.calls)
	}
}

func TestHandleSkipsBLeg(t *testing.T) {
	fs := &fakeStore{}
	b := New(fs, webhook.New("", ""))

	ev := aLeg("CHANNEL_CREATE", nil)
	ev["Unique-ID"] = "uuid-b"
	ev["Channel-Call-UUID"] = "uuid-a" // bridged B-leg points at A-leg
	b.Handle(ev)

	if len(fs.calls) != 0 {
		t.Fatalf("B-leg must be skipped, got %d upserts", len(fs.calls))
	}
}

func TestHandleBackfillsMidCall(t *testing.T) {
	fs := &fakeStore{}
	b := New(fs, webhook.New("", ""))

	// listener reconnected mid-call: first event seen is the hangup
	b.Handle(aLeg("CHANNEL_HANGUP_COMPLETE", map[string]string{
		"Hangup-Cause":     "NORMAL_CLEARING",
		"variable_billsec": "7",
	}))

	if len(fs.calls) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(fs.calls))
	}
	if fs.calls[0].Caller != "1000" || fs.calls[0].DurationSec != 7 {
		t.Errorf("backfill: %+v", fs.calls[0])
	}
}

func TestHandleCarriesTenantIntoWebhookPayload(t *testing.T) {
	// 捕获 webhook 载荷：export 发生在拨号计划期，CHANNEL_CREATE 无租户变量、
	// ANSWER 起带出 → answered/hangup 载荷应含 tenant_id，ringing 不含。
	var payloads []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Event   string         `json:"event"`
			Payload map[string]any `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.Payload["__event"] = body.Event
		payloads = append(payloads, body.Payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs := &fakeStore{}
	b := New(fs, webhook.New(srv.URL, ""))

	b.Handle(aLeg("CHANNEL_CREATE", nil))
	b.Handle(aLeg("CHANNEL_ANSWER", map[string]string{"variable_cc_tenant_id": "2"}))
	b.Handle(aLeg("CHANNEL_HANGUP_COMPLETE", map[string]string{
		"Hangup-Cause": "NORMAL_CLEARING", "variable_billsec": "9",
	}))

	if len(payloads) != 3 {
		t.Fatalf("want 3 webhook payloads, got %d", len(payloads))
	}
	if _, has := payloads[0]["tenant_id"]; has {
		t.Errorf("ringing payload must not carry tenant yet: %+v", payloads[0])
	}
	for _, p := range payloads[1:] {
		if v, _ := p["tenant_id"].(float64); v != 2 {
			t.Errorf("%s payload tenant_id = %v, want 2", p["__event"], p["tenant_id"])
		}
	}
}

func TestHandleCarriesVoiceIDIntoWebhookPayload(t *testing.T) {
	var payloads []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Event   string         `json:"event"`
			Payload map[string]any `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.Payload["__event"] = body.Event
		payloads = append(payloads, body.Payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	b := New(&fakeStore{}, webhook.New(srv.URL, ""))
	b.Handle(aLeg("CHANNEL_CREATE", map[string]string{"variable_cc_voice_id": "9"}))
	b.Handle(aLeg("CHANNEL_HANGUP_COMPLETE", map[string]string{
		"Hangup-Cause": "NO_ANSWER", "variable_cc_voice_id": "9",
	}))
	if len(payloads) != 2 {
		t.Fatalf("payloads=%d", len(payloads))
	}
	for _, p := range payloads {
		if v, _ := p["voice_id"].(float64); v != 9 {
			t.Errorf("%s voice_id=%v want 9", p["__event"], p["voice_id"])
		}
	}
}
