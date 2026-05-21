package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	applogger "github.com/go-admin-kit/server/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorResponsesUseRealHTTPStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		respond    func(*gin.Context)
		wantStatus int
		wantCode   int
		wantErr    ErrorCode
		wantMsg    string
	}{
		{
			name: "bad request",
			respond: func(c *gin.Context) {
				BadRequest(c, "bad input")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   http.StatusBadRequest,
			wantErr:    ErrorCodeBadRequest,
			wantMsg:    "bad input",
		},
		{
			name: "unauthorized",
			respond: func(c *gin.Context) {
				Unauthorized(c, "login required")
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   http.StatusUnauthorized,
			wantErr:    ErrorCodeUnauthorized,
			wantMsg:    "login required",
		},
		{
			name: "forbidden",
			respond: func(c *gin.Context) {
				Forbidden(c, "permission denied")
			},
			wantStatus: http.StatusForbidden,
			wantCode:   http.StatusForbidden,
			wantErr:    ErrorCodeForbidden,
			wantMsg:    "permission denied",
		},
		{
			name: "not found",
			respond: func(c *gin.Context) {
				NotFound(c, "missing")
			},
			wantStatus: http.StatusNotFound,
			wantCode:   http.StatusNotFound,
			wantErr:    ErrorCodeNotFound,
			wantMsg:    "missing",
		},
		{
			name: "internal server error",
			respond: func(c *gin.Context) {
				InternalServerError(c, "database password leaked in driver error")
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   http.StatusInternalServerError,
			wantErr:    ErrorCodeInternalServerError,
			wantMsg:    "internal server error",
		},
		{
			name: "generic http error",
			respond: func(c *gin.Context) {
				Error(c, http.StatusTooManyRequests, "limited")
			},
			wantStatus: http.StatusTooManyRequests,
			wantCode:   http.StatusTooManyRequests,
			wantErr:    ErrorCodeTooManyRequests,
			wantMsg:    "limited",
		},
		{
			name: "service unavailable",
			respond: func(c *gin.Context) {
				Error(c, http.StatusServiceUnavailable, "unavailable")
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   http.StatusServiceUnavailable,
			wantErr:    ErrorCodeServiceUnavailable,
			wantMsg:    "unavailable",
		},
		{
			name: "legacy application code falls back to 500 transport status",
			respond: func(c *gin.Context) {
				Error(c, 10001, "legacy error")
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   10001,
			wantErr:    ErrorCodeInternalServerError,
			wantMsg:    "legacy error",
		},
		{
			name: "domain error code",
			respond: func(c *gin.Context) {
				BadRequestWithCode(c, ErrorCodeUsernameAlreadyExists, "username already exists")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   http.StatusBadRequest,
			wantErr:    ErrorCodeUsernameAlreadyExists,
			wantMsg:    "username already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, body := recordResponse(t, tt.respond)

			if w.Code != tt.wantStatus {
				t.Fatalf("http status = %d, want %d", w.Code, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Fatalf("body code = %d, want %d", body.Code, tt.wantCode)
			}
			if body.Message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", body.Message, tt.wantMsg)
			}
			if body.ErrorCode != tt.wantErr {
				t.Fatalf("error_code = %q, want %q", body.ErrorCode, tt.wantErr)
			}
		})
	}
}

func TestInternalServerErrorMasksResponseAndLogsDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, logs := observer.New(zap.ErrorLevel)
	oldLogger := applogger.Logger
	applogger.Logger = zap.New(core)
	t.Cleanup(func() {
		applogger.Logger = oldLogger
	})

	_, body := recordResponse(t, func(c *gin.Context) {
		InternalServerError(c, "database password=secret failed")
	})

	if body.Message != "internal server error" {
		t.Fatalf("message = %q, want internal server error", body.Message)
	}
	if body.ErrorCode != ErrorCodeInternalServerError {
		t.Fatalf("error_code = %q, want %q", body.ErrorCode, ErrorCodeInternalServerError)
	}
	entries := logs.FilterMessage("internal server error").All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["detail"] != "database password=secret failed" {
		t.Fatalf("logged detail = %#v, want original error detail", fields["detail"])
	}
}

func recordResponse(t *testing.T, respond func(*gin.Context)) (*httptest.ResponseRecorder, Response) {
	t.Helper()

	router := gin.New()
	router.GET("/", respond)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var body Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return w, body
}
