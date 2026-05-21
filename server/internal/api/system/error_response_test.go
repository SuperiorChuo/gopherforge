package system

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
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
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeUsernameAlreadyExists)
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
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeRoleCodeAlreadyExists)
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
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodePermissionCodeAlreadyExists)
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
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeMenuParentNotFound)
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
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeDepartmentCodeAlreadyExists)
}

func TestSystemDictServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemDictServiceError(c, "failed to create dict type", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemDictServiceErrorAllowsKnownDictErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemDictServiceError(c, "failed to create dict type", systemsvc.ErrDictTypeCodeAlreadyExists)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "dict type code already exists") {
		t.Fatalf("response did not include safe dict error message: %s", recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeDictTypeCodeAlreadyExists)
}

func TestSystemNoticeServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemNoticeServiceError(c, "failed to create notice", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemNoticeServiceErrorAllowsKnownNoticeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemNoticeServiceError(c, "failed to get notice", systemsvc.ErrNoticeNotFound)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), "notice not found") {
		t.Fatalf("response did not include safe notice error message: %s", recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeNoticeNotFound)
}

func TestSystemFileServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemFileServiceError(c, "failed to upload file", errors.New("open C:\\secret\\upload.tmp: access denied"))

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
	if strings.Contains(recorder.Body.String(), "C:\\secret") || strings.Contains(recorder.Body.String(), "access denied") {
		t.Fatalf("response leaked internal error details: %s", recorder.Body.String())
	}
}

func TestSystemFileServiceErrorAllowsKnownFileErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemFileServiceError(c, "failed to get file", systemsvc.ErrFileNotFoundOrPermissionDenied)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), "file not found or permission denied") {
		t.Fatalf("response did not include safe file error message: %s", recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeFileNotFoundOrPermissionDenied)
}

func TestSystemOperationLogServiceErrorHidesUnexpectedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemOperationLogServiceError(c, "failed to get operation log", errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

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

func TestSystemOperationLogServiceErrorAllowsKnownLogErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeSystemOperationLogServiceError(c, "failed to get operation log", systemsvc.ErrOperationLogNotFound)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), "operation log not found") {
		t.Fatalf("response did not include safe operation log error message: %s", recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeOperationLogNotFound)
}

func assertErrorCode(t *testing.T, body []byte, want response.ErrorCode) {
	t.Helper()

	var payload struct {
		ErrorCode response.ErrorCode `json:"error_code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ErrorCode != want {
		t.Fatalf("error_code = %q, want %q; body=%s", payload.ErrorCode, want, string(body))
	}
}
