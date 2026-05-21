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

func TestSystemRoleServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemRoleServiceError(c, "failed to create role", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemRoleServiceErrorAllowsKnownRoleErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemRoleServiceError(c, "failed to create role", systemsvc.ErrRoleCodeAlreadyExists)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "role code already exists") {
		t.Fatalf("response did not include safe role error message: %s", recorder.Body.String())
	}
}

func TestSystemPermissionServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemPermissionServiceError(c, "failed to create permission", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemPermissionServiceErrorAllowsKnownPermissionErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemPermissionServiceError(c, "failed to create permission", systemsvc.ErrPermissionCodeAlreadyExists)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "permission code already exists") {
		t.Fatalf("response did not include safe permission error message: %s", recorder.Body.String())
	}
}

func TestSystemMenuServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemMenuServiceError(c, "failed to create menu", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemMenuServiceErrorAllowsKnownMenuErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemMenuServiceError(c, "failed to create menu", systemsvc.ErrParentMenuNotFound)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "parent menu not found") {
		t.Fatalf("response did not include safe menu error message: %s", recorder.Body.String())
	}
}

func TestSystemDepartmentServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemDepartmentServiceError(c, "failed to create department", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemDepartmentServiceErrorAllowsKnownDepartmentErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemDepartmentServiceError(c, "failed to create department", systemsvc.ErrDepartmentCodeAlreadyExists)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "department code already exists") {
		t.Fatalf("response did not include safe department error message: %s", recorder.Body.String())
	}
}
