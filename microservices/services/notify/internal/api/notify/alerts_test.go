package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/notify/internal/model"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
	"gorm.io/gorm"
)

func newAlertsServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db, "inbox")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	srv := &Server{Store: st, InternalToken: "tkn", AlertUserID: 1}
	r := gin.New()
	r.POST("/api/v1/notify/internal/alerts", srv.ReceiveAlerts)
	return srv, r
}

func postAlerts(r http.Handler, body, tokenHeader, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notify/internal/alerts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tokenHeader != "" {
		req.Header.Set("X-Internal-Token", tokenHeader)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const amPayload = `{"status":"firing","alerts":[{
  "status":"firing",
  "labels":{"alertname":"ServiceDown","severity":"critical","job":"go-admin-kit-services","service":"cc"},
  "annotations":{"summary":"cc-service target down"},
  "startsAt":"2026-07-23T10:00:00Z",
  "fingerprint":"abc123"}]}`

func TestReceiveAlertsAuthDualTrack(t *testing.T) {
	_, r := newAlertsServer(t)
	if w := postAlerts(r, amPayload, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: %d, want 401", w.Code)
	}
	if w := postAlerts(r, amPayload, "wrong", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d, want 401", w.Code)
	}
	if w := postAlerts(r, amPayload, "", "tkn"); w.Code != http.StatusOK {
		t.Fatalf("bearer auth: %d body=%s", w.Code, w.Body.String())
	}
}

func TestReceiveAlertsCreatesInboxAndDedupes(t *testing.T) {
	srv, r := newAlertsServer(t)

	if w := postAlerts(r, amPayload, "tkn", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"created":1`) {
		t.Fatalf("first post: %d body=%s", w.Code, w.Body.String())
	}
	// Alertmanager repeat_interval 重复推送：同 fingerprint+status+startsAt 去重。
	if w := postAlerts(r, amPayload, "tkn", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"skipped":1`) {
		t.Fatalf("repeat post: %d body=%s", w.Code, w.Body.String())
	}
	// 恢复是新 ref，单独通知。
	resolved := strings.ReplaceAll(amPayload, `"status":"firing"`, `"status":"resolved"`)
	if w := postAlerts(r, resolved, "tkn", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"created":1`) {
		t.Fatalf("resolved post: %d body=%s", w.Code, w.Body.String())
	}

	var msgs []model.Message
	srv.Store.DB().Order("id ASC").Find(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2 (firing + resolved)", len(msgs))
	}
	if msgs[0].TemplateCode != "ops_alert" || msgs[1].TemplateCode != "ops_alert" {
		t.Fatalf("template codes = %q/%q, want ops_alert", msgs[0].TemplateCode, msgs[1].TemplateCode)
	}
	if !strings.Contains(msgs[0].Title, "告警") || !strings.Contains(msgs[0].Title, "ServiceDown") {
		t.Fatalf("firing title = %q", msgs[0].Title)
	}
	// 正文走 ops_alert 模板（summary 字段；Alertmanager 载荷普遍只带 summary 不带 description）。
	if !strings.Contains(msgs[0].Content, "cc-service target down") {
		t.Fatalf("firing content = %q, want summary detail", msgs[0].Content)
	}
	if !strings.Contains(msgs[1].Title, "ServiceDown") {
		t.Fatalf("resolved title = %q", msgs[1].Title)
	}
}
