package system

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	systemsvc "github.com/go-admin-kit/server/internal/service/system"
)

func TestSystemServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemUserServiceError(c, "failed to create user", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemServiceErrorAllowsKnownUserErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemUserServiceError(c, "failed to create user", systemsvc.ErrUsernameAlreadyExists)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "username already exists") {
		t.Fatalf("response did not include safe user error message: %s", recorder.Body.String())
	}
}
