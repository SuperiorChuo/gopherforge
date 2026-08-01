package system

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalSendSmsFailsClosedWithoutConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/v1/sms/send", NewSmsAPI().InternalSendSms(""))
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/sms/send", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestInternalSendSmsRejectsWrongTokenBeforeBodyOrDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/v1/sms/send", NewSmsAPI().InternalSendSms("expected-token"))
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/sms/send", strings.NewReader(`{}`))
	req.Header.Set("X-Internal-Token", "wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
