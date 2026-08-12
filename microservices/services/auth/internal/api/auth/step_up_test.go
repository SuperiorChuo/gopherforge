package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	authmiddleware "github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

func TestEdgeCertificateExportStepUpAPIReturnsNoStoreProof(t *testing.T) {
	fake := &fakeEdgeCertificateExportStepUpVerifier{
		response: &authsvc.EdgeCertificateExportStepUpResponse{Proof: "opaque-proof", ExpiresInSeconds: 120},
	}
	recorder := callEdgeCertificateStepUp(t, fake, true, `{"current_password":"CurrentPass1","totp_code":"123456","certificate_id":42}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	assertStepUpSecurityHeaders(t, recorder)
	if fake.userID != 7 || fake.sessionID != "session-123" || fake.request.CertificateID != 42 || fake.request.CurrentPassword != "CurrentPass1" || fake.request.TOTPCode != "123456" {
		t.Fatalf("service call = user %d session %q request %#v", fake.userID, fake.sessionID, fake.request)
	}
	if strings.Contains(recorder.Body.String(), "CurrentPass1") || strings.Contains(recorder.Body.String(), "123456") {
		t.Fatalf("response leaked submitted factors: %s", recorder.Body.String())
	}
}

func TestEdgeCertificateExportStepUpAPIUsesOneGenericCredentialError(t *testing.T) {
	for _, serviceErr := range []error{
		authsvc.ErrStepUpVerificationFailed,
		fmtWrappedError{inner: authsvc.ErrStepUpVerificationFailed},
	} {
		recorder := callEdgeCertificateStepUp(t, &fakeEdgeCertificateExportStepUpVerifier{err: serviceErr}, true, `{"current_password":"wrong","certificate_id":42}`)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
		}
		var payload response.Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload.Message != "step-up verification failed" {
			t.Fatalf("message = %q, want generic step-up failure", payload.Message)
		}
	}
}

func TestEdgeCertificateExportStepUpAPIFailsClosedAndRequiresPlatformAdmin(t *testing.T) {
	unavailable := callEdgeCertificateStepUp(t, &fakeEdgeCertificateExportStepUpVerifier{err: authsvc.ErrStepUpUnavailable}, true, `{"current_password":"CurrentPass1","certificate_id":42}`)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable response = status %d headers %#v body=%s", unavailable.Code, unavailable.Header(), unavailable.Body.String())
	}
	assertStepUpSecurityHeaders(t, unavailable)

	denied := callEdgeCertificateStepUp(t, &fakeEdgeCertificateExportStepUpVerifier{}, false, `{"current_password":"CurrentPass1","certificate_id":42}`)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-platform-admin status = %d, want 403; body=%s", denied.Code, denied.Body.String())
	}
	assertStepUpSecurityHeaders(t, denied)
}

func TestEdgeCertificateExportStepUpAPIReturnsGenericRateLimit(t *testing.T) {
	recorder := callEdgeCertificateStepUp(t, &fakeEdgeCertificateExportStepUpVerifier{err: authsvc.ErrStepUpRateLimited}, true, `{"current_password":"CurrentPass1","totp_code":"123456","certificate_id":42}`)
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "300" {
		t.Fatalf("rate limit response = status %d Retry-After %q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "password") || strings.Contains(recorder.Body.String(), "totp") {
		t.Fatalf("rate limit response identified a rejected factor: %s", recorder.Body.String())
	}
}

func TestEdgeCertificateExportStepUpAPIRejectsMalformedRequestWithoutEcho(t *testing.T) {
	recorder := callEdgeCertificateStepUp(t, &fakeEdgeCertificateExportStepUpVerifier{}, true, `{"current_password":"super-secret"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "super-secret") || strings.Contains(recorder.Body.String(), "CertificateID") {
		t.Fatalf("binding error leaked request details: %s", recorder.Body.String())
	}
}

type fakeEdgeCertificateExportStepUpVerifier struct {
	response  *authsvc.EdgeCertificateExportStepUpResponse
	err       error
	userID    uint
	sessionID string
	request   authsvc.EdgeCertificateExportStepUpRequest
}

func (f *fakeEdgeCertificateExportStepUpVerifier) VerifyAndIssueContext(_ context.Context, userID uint, sessionID string, req authsvc.EdgeCertificateExportStepUpRequest) (*authsvc.EdgeCertificateExportStepUpResponse, error) {
	f.userID = userID
	f.sessionID = sessionID
	f.request = req
	return f.response, f.err
}

func callEdgeCertificateStepUp(t *testing.T, service edgeCertificateExportStepUpVerifier, platformAdmin bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/step-up", func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("session_id", "session-123")
		c.Set("platform_admin", platformAdmin)
		c.Next()
	}, edgeCertificateExportNoStoreHeaders(), authmiddleware.PlatformAdminMiddleware(), (&EdgeCertificateExportStepUpAPI{service: service}).IssueEdgeCertificateExportProof)
	req := httptest.NewRequest(http.MethodPost, "/step-up", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func assertStepUpSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	want := map[string]string{
		"Cache-Control":          "no-store, private, max-age=0",
		"Pragma":                 "no-cache",
		"Expires":                "0",
		"Vary":                   "Authorization, Cookie",
		"X-Content-Type-Options": "nosniff",
	}
	for name, value := range want {
		if got := recorder.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

type fmtWrappedError struct{ inner error }

func (e fmtWrappedError) Error() string { return "wrapped: " + e.inner.Error() }
func (e fmtWrappedError) Unwrap() error { return e.inner }
