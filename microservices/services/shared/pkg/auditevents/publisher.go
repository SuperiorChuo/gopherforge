// Package auditevents 发布数据变更审计事件到 NATS（Phase 2D 审计事件化）。

package auditevents

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type AuditEvent struct {
	TenantID   uint           `json:"tenant_id"`
	ActorType  string         `json:"actor_type"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Before     map[string]any `json:"before,omitempty"`
	After      map[string]any `json:"after,omitempty"`
	Summary    string         `json:"summary"`
	CreatedAt  time.Time      `json:"created_at"`
}

var (
	mu     sync.RWMutex
	js     nats.JetStreamContext
	initMu sync.Mutex
)

func Init(url string) error {
	if url == "" {
		return nil
	}
	initMu.Lock()
	defer initMu.Unlock()
	if js != nil {
		return nil
	}
	nc, err := nats.Connect(url,
		nats.Name("audit-events"),
		nats.MaxReconnects(-1),
		nats.Timeout(3*time.Second),
	)
	if err != nil {
		return err
	}
	jet, err := nc.JetStream(nats.PublishAsyncMaxPending(4096))
	if err != nil {
		nc.Close()
		return err
	}
	_, err = jet.StreamInfo("audit_events")
	if err != nil {
		_, _ = jet.AddStream(&nats.StreamConfig{
			Name:      "audit_events",
			Subjects:  []string{"audit.log.>"},
			Storage:   nats.FileStorage,
			Retention: nats.LimitsPolicy,
			MaxAge:    7 * 24 * time.Hour,
		})
	}
	mu.Lock()
	js = jet
	mu.Unlock()
	return nil
}

func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return js != nil
}

func PublishOperationLog(service string, opLog any) {
	data, err := json.Marshal(opLog)
	if err != nil {
		auditLogf("auditevents: marshal operation log failed: %v", err)
		return
	}
	mu.RLock()
	j := js
	mu.RUnlock()
	if j == nil {
		return
	}
	subject := "operation.log." + service
	if _, err := j.PublishAsync(subject, data); err != nil {
		auditLogf("auditevents: publish %s failed: %v", subject, err)
	}
}

func PublishRaw(subject string, data []byte) error {
	mu.RLock()
	j := js
	mu.RUnlock()
	if j == nil {
		return nil
	}
	if _, err := j.PublishAsync(subject, data); err != nil {
		return err
	}
	return nil
}

func Publish(ev *AuditEvent) {
	if ev == nil {
		return
	}
	data, err := json.Marshal(ev)
	if err != nil {
		auditLogf("auditevents: marshal failed: %v", err)
		return
	}
	mu.RLock()
	j := js
	mu.RUnlock()
	if j == nil {
		auditLogf("auditevents: 未初始化（NATS 未配置），跳过审计事件 %s/%s", ev.TargetType, ev.Action)
		return
	}
	subject := "audit.log." + ev.Action
	if _, err := j.PublishAsync(subject, data); err != nil {
		auditLogf("auditevents: publish %s failed: %v", subject, err)
	}
}

var auditLogWriter io.Writer = os.Stderr

func SetLogWriter(w io.Writer) {
	if w != nil {
		auditLogWriter = w
	}
}

func auditLogf(format string, a ...any) {
	fmt.Fprintf(auditLogWriter, "[auditevents] "+format+"\n", a...)
}
