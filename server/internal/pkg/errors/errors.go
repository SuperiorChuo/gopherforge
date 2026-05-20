package errors

import (
	"fmt"
	"net/http"
)

// AppError 应用错误
type AppError struct {
	Code    int
	Message string
	Err     error
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap 返回底层错误
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError 创建应用错误
func NewAppError(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// NewAppErrorWithErr 创建带底层错误的应用错误
func NewAppErrorWithErr(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// 预定义错误
var (
	ErrInvalidInput  = NewAppError(400, "invalid input")
	ErrUnauthorized  = NewAppError(401, "unauthorized")
	ErrForbidden     = NewAppError(403, "forbidden")
	ErrNotFound      = NewAppError(404, "not found")
	ErrInternalError = NewAppError(500, "internal server error")
)

// GetHTTPStatus 获取 HTTP 状态码
func GetHTTPStatus(code int) int {
	switch code {
	case 400:
		return http.StatusBadRequest
	case 401:
		return http.StatusUnauthorized
	case 403:
		return http.StatusForbidden
	case 404:
		return http.StatusNotFound
	case 500:
		return http.StatusInternalServerError
	default:
		return http.StatusOK
	}
}
