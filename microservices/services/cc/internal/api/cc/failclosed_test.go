package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 生产环境缺 CC_WEBHOOK_SECRET / CC_CDR_AUTH 时，端点必须直接拒绝请求。
// 老行为是"空密钥即跳过鉴权"（fail open），等于把写话单/写账的入口敞开。
// 启动不再被阻断，所以这层拒绝是唯一的兜底。

func TestWebhookRefusedInProductionWithoutSecret(t *testing.T) {
	srv, r := newTestServer(t, "")
	srv.ProductionMode = true

	w := postWebhook(r, envelope("call.hangup", "call-1", nil), "")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when CC_WEBHOOK_SECRET is unset in production", w.Code)
	}
	// 事件不得落库——拒绝要发生在任何副作用之前。
	var n int64
	srv.Store.DB().Table("cc_webhook_logs").Count(&n)
	if n != 0 {
		t.Fatalf("webhook logs = %d, want 0 — refused requests must not be persisted", n)
	}
}

// 开发环境保持原有便利：空密钥仍然免验签。
func TestWebhookStillOpenOutsideProductionWithoutSecret(t *testing.T) {
	_, r := newTestServer(t, "")

	if w := postWebhook(r, envelope("call.hangup", "call-2", nil), ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for development without a secret", w.Code)
	}
}

// 配了密钥时，生产环境照常验签：签名对就放行，不对就 401。
func TestWebhookVerifiesSignatureInProduction(t *testing.T) {
	const secret = "test-cc-webhook-token"
	srv, r := newTestServer(t, secret)
	srv.ProductionMode = true
	body := envelope("call.hangup", "call-3", nil)

	if w := postWebhook(r, body, sign(secret, body)); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a correctly signed request", w.Code)
	}
	if w := postWebhook(r, body, "sha256=deadbeef"); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a bad signature", w.Code)
	}
}

func TestCDRRefusedInProductionWithoutCredentials(t *testing.T) {
	srv, r := newTestServerWithCDR(t)
	srv.ProductionMode = true

	w := postCDR(t, r, map[string]string{"uuid": "cdr-1"})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when CC_CDR_AUTH is unset in production", w.Code)
	}
}

func TestCDRStillOpenOutsideProductionWithoutCredentials(t *testing.T) {
	_, r := newTestServerWithCDR(t)

	if w := postCDR(t, r, map[string]string{"uuid": "cdr-2"}); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for development without credentials", w.Code)
	}
}

// 配了凭据时，生产环境照常校验 Basic。
func TestCDRChecksBasicAuthInProduction(t *testing.T) {
	srv, r := newTestServerWithCDR(t)
	srv.ProductionMode = true
	srv.CDRAuth = "fsuser:9c2e7a41f0"

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cc/cdr", bytes.NewReader([]byte(`{"variables":{"uuid":"cdr-3"}}`)))
	req.SetBasicAuth("fsuser", "9c2e7a41f0")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for correct credentials", w.Code)
	}

	if w := postCDR(t, r, map[string]string{"uuid": "cdr-4"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing credentials", w.Code)
	}
}
