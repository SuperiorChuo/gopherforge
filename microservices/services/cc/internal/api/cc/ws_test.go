package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/hub"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// newTestServerWithHub 复用 webhook 路由，但额外挂 hub 并预置一条联系人，
// 用于断言推送挂钩（含 caller_name 富化）。
func newTestServerWithHub(t *testing.T) (*Server, *gin.Engine, *hub.Hub) {
	t.Helper()
	srv, r := newTestServer(t, "")
	h := hub.New()
	srv.Hub = h
	// 预置联系人：入方向来电按主叫号命中，弹屏显名。
	if _, err := srv.Store.UpsertContact(context.Background(), &store.Contact{
		TenantID: 1, Number: "13800009999", Name: "张三测试",
	}); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return srv, r, h
}

func TestWebhookPushEnrichesCallerName(t *testing.T) {
	srv, r, h := newTestServerWithHub(t)
	_ = srv

	ch, unregister := h.Register(1, 1)
	defer unregister()

	body := envelope("call.ringing", "call-ws-1", map[string]any{
		"caller": "13800009999", "callee": "5000", "direction": "inbound",
	})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	select {
	case b := <-ch:
		var ev hub.Event
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Type != "call.ringing" {
			t.Fatalf("type = %q, want call.ringing", ev.Type)
		}
		if ev.CallID != "call-ws-1" || ev.Caller != "13800009999" || ev.Direction != "inbound" {
			t.Fatalf("event fields wrong: %+v", ev)
		}
		if ev.CallerName != "张三测试" {
			t.Fatalf("caller_name = %q, want 张三测试 (contact enrichment)", ev.CallerName)
		}
		if ev.TS == 0 {
			t.Fatal("ts must be set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no ws event pushed within 2s")
	}
}

func TestWebhookPushWithoutContactLeavesNameEmpty(t *testing.T) {
	srv, r, h := newTestServerWithHub(t)
	_ = srv

	ch, unregister := h.Register(1, 1)
	defer unregister()

	// 未登记的号码：caller_name 应留空，不阻塞推送。
	body := envelope("call.answered", "call-ws-2", map[string]any{
		"caller": "13900001111", "agent_ext": "1000", "direction": "inbound",
	})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	select {
	case b := <-ch:
		var ev hub.Event
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Type != "call.answered" || ev.CallerName != "" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no ws event pushed within 2s")
	}
}

// recording.ready 等非实时事件不推 WS。
func TestWebhookNonRealtimeNotPushed(t *testing.T) {
	_, r, h := newTestServerWithHub(t)
	ch, unregister := h.Register(1, 1)
	defer unregister()

	body := envelope("recording.ready", "call-ws-3", map[string]any{"recording": "/r/x.wav"})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	select {
	case b := <-ch:
		t.Fatalf("recording.ready must not push, got %s", string(b))
	case <-time.After(300 * time.Millisecond):
		// 符合预期：无推送
	}
}
