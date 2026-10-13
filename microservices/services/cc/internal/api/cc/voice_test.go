package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

func newVoiceRouter(t *testing.T) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&store.CCRate{}, &store.CCBillingAccount{},
		&store.CCRiskPolicy{}, &store.CCRiskListEntry{}, &store.CCRiskAttempt{},
		&store.CCVoiceNotify{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), Secret: "test-jwt-secret", InternalToken: "itok"}
	r := gin.New()
	r.POST("/api/v1/cc/internal/voice-notify", srv.InternalVoiceNotify)
	r.POST("/api/v1/cc/voice", srv.CreateVoiceNotify)
	r.GET("/api/v1/cc/voice", srv.ListVoiceNotifies)
	return r, srv
}

func TestInternalRiskCheckAllowAndDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&store.CCRate{}, &store.CCBillingAccount{}, &store.CCRiskPolicy{}, &store.CCRiskListEntry{}, &store.CCRiskAttempt{}, &store.Agent{}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store.NewWithDB(db), WebhookSecret: "s3cret"}
	r := gin.New()
	r.POST("/api/v1/cc/internal/risk-check", srv.InternalRiskCheck)

	body := `{"from_ext":"1000","to":"13800138000"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/internal/risk-check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no sig: %d", w.Code)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cc/internal/risk-check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CC-Signature", sign("s3cret", []byte(body)))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"allow":true`) {
		t.Fatalf("allow: %d %s", w.Code, w.Body.String())
	}

	if _, err := srv.Store.CreateRiskList(context.Background(), &store.CCRiskListEntry{
		TenantID: 1, Number: "13800138000", Match: "exact", ListType: "black",
	}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cc/internal/risk-check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CC-Signature", sign("s3cret", []byte(body)))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("deny: %d %s", w.Code, w.Body.String())
	}
}

func TestVoiceNotifyEnqueueAndList(t *testing.T) {
	r, _ := newVoiceRouter(t)
	w := httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/voice",
		strings.NewReader(`{"to":"13800138000","text":"您的工单即将超时"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/voice", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatalf("list: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestVoiceNotifyRiskDeniedDoesNotEnqueue(t *testing.T) {
	r, srv := newVoiceRouter(t)
	if _, err := srv.Store.CreateRiskList(context.Background(), &store.CCRiskListEntry{
		TenantID: 1, Number: "13800138000", Match: "exact", ListType: "black",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := httptest.NewRecorder()
	post := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/voice",
		strings.NewReader(`{"to":"13800138000","text":"hi"}`)))
	post.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, post)
	if w.Code != http.StatusForbidden {
		t.Fatalf("denied: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestInternalVoiceNotifyAuth(t *testing.T) {
	r, srv := newVoiceRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/internal/voice-notify",
		strings.NewReader(`{"to":"13800138000","text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: code=%d", w.Code)
	}

	srv.InternalToken = ""
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled: code=%d", w.Code)
	}

	srv.InternalToken = "itok"
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cc/internal/voice-notify",
		strings.NewReader(`{"to":"13800138000","text":"工单超时提醒"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", "itok")
	req.Header.Set("X-Tenant-ID", "2")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ok: code=%d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || env.Data.ID == 0 {
		t.Fatalf("decode: %v %s", err, w.Body.String())
	}
}

func TestWebhookHangupCompletesVoiceJob(t *testing.T) {
	srv, r := newTestServer(t, "")
	job := &store.CCVoiceNotify{TenantID: 1, To: "13800138000", Text: "hi"}
	if err := srv.Store.CreateVoiceNotify(context.Background(), job); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = srv.Store.FinishVoiceNotify(context.Background(), job.ID, "queued", "vn1.wav", "+OK", "")

	body := envelope("call.hangup", "uuid-voice-1", map[string]any{
		"voice_id": float64(job.ID), "duration_sec": float64(12), "cause": "NORMAL_CLEARING",
	})
	if w := postWebhook(r, body, ""); w.Code != http.StatusOK {
		t.Fatalf("hangup: %d %s", w.Code, w.Body.String())
	}
	got, err := srv.Store.GetVoiceNotify(context.Background(), 1, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "completed" || got.CallID != "uuid-voice-1" {
		t.Fatalf("got %+v", got)
	}
	// 迟到 ringing 不回退
	_ = postWebhook(r, envelope("call.answered", "uuid-voice-1", map[string]any{"voice_id": float64(job.ID)}), "")
	got, _ = srv.Store.GetVoiceNotify(context.Background(), 1, job.ID)
	if got.Status != "completed" {
		t.Fatalf("regressed to %s", got.Status)
	}
}

func TestWebhookHangupNoAnswer(t *testing.T) {
	srv, r := newTestServer(t, "")
	job := &store.CCVoiceNotify{TenantID: 1, To: "13800138000", Text: "hi"}
	if err := srv.Store.CreateVoiceNotify(context.Background(), job); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = srv.Store.FinishVoiceNotify(context.Background(), job.ID, "queued", "", "", "")
	body := envelope("call.hangup", "uuid-na", map[string]any{
		"voice_id": float64(job.ID), "cause": "NO_ANSWER",
	})
	_ = postWebhook(r, body, "")
	got, _ := srv.Store.GetVoiceNotify(context.Background(), 1, job.ID)
	if got.Status != "noanswer" {
		t.Fatalf("status=%s want noanswer", got.Status)
	}
}

func TestCDRHangupCompletesVoiceJob(t *testing.T) {
	srv, r := newTestServerWithCDR(t)
	job := &store.CCVoiceNotify{TenantID: 1, To: "13800138000", Text: "hi"}
	if err := srv.Store.CreateVoiceNotify(context.Background(), job); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = srv.Store.FinishVoiceNotify(context.Background(), job.ID, "queued", "", "", "")
	w := postCDR(t, r, map[string]string{
		"uuid": "cdr-voice-1", "direction": "outbound", "billsec": "8",
		"hangup_cause": "NORMAL_CLEARING", "cc_voice_id": fmt.Sprintf("%d", job.ID),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("cdr: %d %s", w.Code, w.Body.String())
	}
	got, _ := srv.Store.GetVoiceNotify(context.Background(), 1, job.ID)
	if got.Status != "completed" || got.CallID != "cdr-voice-1" {
		t.Fatalf("got %+v", got)
	}
}
