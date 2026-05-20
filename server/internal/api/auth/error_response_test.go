package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	authsvc "github.com/go-admin-kit/server/internal/service/auth"
)

func TestAuthServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeAuthServiceError(c, "login failed", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message != "internal server error" {
		t.Fatalf("message = %q, want internal server error", payload.Message)
	}
	if strings.Contains(recorder.Body.String(), "10.0.0.5") || strings.Contains(recorder.Body.String(), "dial tcp") {
		t.Fatalf("response leaked internal error details: %s", recorder.Body.String())
	}
}

func TestAuthServiceErrorAllowsKnownLoginFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeAuthServiceError(c, "login failed", authsvc.ErrInvalidCredentials)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(recorder.Body.String(), "invalid username or password") {
		t.Fatalf("response did not include safe auth failure message: %s", recorder.Body.String())
	}
}

func TestJWTUnauthorizedErrorUsesStableMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeJWTUnauthorizedError(c, jwt.ErrExpiredToken)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(recorder.Body.String(), "Token has expired") {
		t.Fatalf("response did not include stable expired-token message: %s", recorder.Body.String())
	}
}
