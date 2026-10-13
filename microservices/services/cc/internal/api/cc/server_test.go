package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

func newTestServer(t *testing.T, secret string) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.Call{}, &store.WebhookLog{}, &store.Agent{}, &store.Contact{}, &store.CCVoiceNotify{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), WebhookSecret: secret, Secret: "test-jwt-secret"}
	r := gin.New()
	r.POST("/api/v1/cc/webhook", srv.Webhook)
	r.GET("/api/v1/cc/calls", srv.ListCalls)
	r.GET("/api/v1/cc/lookup", srv.Lookup)
	r.GET("/api/v1/cc/agents", srv.ListAgents)
	r.PUT("/api/v1/cc/agents/:user_id", srv.BindAgent)
	r.DELETE("/api/v1/cc/agents/:user_id", srv.UnbindAgent)
	r.GET("/api/v1/cc/contacts", srv.ListContacts)
	r.POST("/api/v1/cc/contacts", srv.UpsertContact)
	r.DELETE("/api/v1/cc/contacts/:number", srv.DeleteContact)
	r.GET("/api/v1/cc/cid", srv.CidLookup)
	r.GET("/api/v1/cc/reports/overview", srv.ReportsOverview)
	return srv, r
}

// newTestServerWithCDR 额外挂上 /cc/cdr 路由（CDRAuth 空=放行），用于挂机跟进挂钩测试。
func newTestServerWithCDR(t *testing.T) (*Server, *gin.Engine) {
	srv, r := newTestServer(t, "")
	r.POST("/api/v1/cc/cdr", srv.ReceiveCDR)
	return srv, r
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func postWebhook(r *gin.Engine, body []byte, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-CC-Signature", sig)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func envelope(event, callID string, extra map[string]any) []byte {
	payload := map[string]any{"call_id": callID}
	for k, v := range extra {
		payload[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"event": event, "source": "go-freeswitch-cc", "payload": payload})
	return raw
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	_, r := newTestServer(t, "s3cret")
	body := envelope("call.ringing", "abc", nil)

	if w := postWebhook(r, body, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no signature: status = %d, want 401", w.Code)
	}
	if w := postWebhook(r, body, "sha256=deadbeef"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: status = %d, want 401", w.Code)
	}
	if w := postWebhook(r, body, sign("s3cret", body)); w.Code != http.StatusOK {
		t.Fatalf("good signature: status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestWebhookLifecycleMergesIntoSingleCall(t *testing.T) {
	srv, r := newTestServer(t, "")

	events := [][]byte{
		envelope("call.ringing", "call-1", map[string]any{"caller": "13800000000", "callee": "5000", "direction": "inbound"}),
		envelope("call.answered", "call-1", map[string]any{"agent_ext": "1000"}),
		envelope("call.hangup", "call-1", map[string]any{"duration_sec": float64(42), "cause": "NORMAL_CLEARING"}),
		envelope("recording.ready", "call-1", map[string]any{"recording": "/recordings/call-1.wav"}),
	}
	for i, body := range events {
		if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
			t.Fatalf("event %d: status = %d body=%s", i, w.Code, w.Body.String())
		}
	}

	list, total, err := srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (events must merge by call_id)", total)
	}
	call := list[0]
	if call.Status != "hangup" {
		t.Fatalf("status = %q, want hangup (recording.ready must not regress status)", call.Status)
	}
	if call.Caller != "13800000000" || call.AgentExt != "1000" || call.DurationSec != 42 {
		t.Fatalf("merged call missing fields: %+v", call)
	}
	if call.Recording != "/recordings/call-1.wav" {
		t.Fatalf("recording = %q", call.Recording)
	}
	if call.StartedAt == nil || call.AnsweredAt == nil || call.EndedAt == nil {
		t.Fatalf("timeline fields missing: %+v", call)
	}
}

func TestWebhookOutOfOrderStatusDoesNotRegress(t *testing.T) {
	srv, r := newTestServer(t, "")

	_ = postWebhook(r, envelope("call.hangup", "call-2", map[string]any{"duration_sec": float64(5)}), "")
	_ = postWebhook(r, envelope("call.ringing", "call-2", nil), "") // 迟到的 ringing

	list, _, err := srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list[0].Status != "hangup" {
		t.Fatalf("status = %q, want hangup (late ringing must not regress)", list[0].Status)
	}
}

func TestWebhookTestEventStoredAsLogOnly(t *testing.T) {
	srv, r := newTestServer(t, "")
	if w := postWebhook(r, envelope("call.test", "t-1", nil), ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	_, total, _ := srv.Store.ListCalls(nil, store.ListCallsQuery{TenantID: 1})
	if total != 0 {
		t.Fatalf("call.test must not create call rows, got %d", total)
	}
	var logs int64
	srv.Store.DB().Model(&store.WebhookLog{}).Count(&logs)
	if logs != 1 {
		t.Fatalf("webhook log rows = %d, want 1", logs)
	}
}

func TestListCallsRequiresAuth(t *testing.T) {
	_, r := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cc/calls", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}

	// 网关注入头即可通过
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cc/calls", nil)
	req.Header.Set("X-Auth-User-ID", "1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("gateway header auth: status = %d body=%s", w.Code, w.Body.String())
	}
}
