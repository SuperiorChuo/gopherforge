package errors

import (
	"fmt"
	"net/http"
)

// AppError is an application error with an HTTP-like code.
type AppError struct {
	Code    int
	Message string
	Err     error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates an application error.
func NewAppError(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// NewAppErrorWithErr creates an application error with an underlying error.
func NewAppErrorWithErr(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Predefined application errors.
var (
	ErrInvalidInput  = NewAppError(400, "invalid input")
	ErrUnauthorized  = NewAppError(401, "unauthorized")
	ErrForbidden     = NewAppError(403, "forbidden")
	ErrNotFound      = NewAppError(404, "not found")
	ErrInternalError = NewAppError(500, "internal server error")
)

// GetHTTPStatus maps an application code to an HTTP status.
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
