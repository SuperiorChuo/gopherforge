package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
)

// Response is the standard API response shape.
type Response struct {
	Code      int       `json:"code"`
	Message   string    `json:"message"`
	ErrorCode ErrorCode `json:"error_code,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// PageResponse is the paginated API response shape.
type PageResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Data     any    `json:"data,omitempty"`
	Total    int64  `json:"total,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// Success writes a successful response.
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: "success",
		Data:    data,
	})
}

// SuccessWithMessage writes a successful response with a custom message.
func SuccessWithMessage(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: message,
		Data:    data,
	})
}

// Error writes an error response with a custom code.
func Error(c *gin.Context, code int, message string) {
	ErrorWithCode(c, code, "", message)
}

// ErrorWithCode writes an error response with a stable machine-readable code.
func ErrorWithCode(c *gin.Context, code int, errorCode ErrorCode, message string) {
	status := httpStatusFromCode(code)
	if errorCode == "" {
		errorCode = defaultErrorCodeForHTTPStatus(status)
	}
	c.JSON(status, Response{
		Code:      code,
		Message:   message,
		ErrorCode: errorCode,
	})
}

// BadRequest writes a 400 response.
func BadRequest(c *gin.Context, message string) {
	BadRequestWithCode(c, ErrorCodeBadRequest, message)
}

// BadRequestWithCode writes a 400 response with a stable machine-readable code.
func BadRequestWithCode(c *gin.Context, errorCode ErrorCode, message string) {
	c.JSON(http.StatusBadRequest, Response{
		Code:      http.StatusBadRequest,
		Message:   message,
		ErrorCode: errorCode,
	})
}

// Unauthorized writes a 401 response.
func Unauthorized(c *gin.Context, message string) {
	UnauthorizedWithCode(c, ErrorCodeUnauthorized, message)
}

// UnauthorizedWithCode writes a 401 response with a stable machine-readable code.
func UnauthorizedWithCode(c *gin.Context, errorCode ErrorCode, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:      http.StatusUnauthorized,
		Message:   message,
		ErrorCode: errorCode,
	})
}

// Forbidden writes a 403 response.
func Forbidden(c *gin.Context, message string) {
	ForbiddenWithCode(c, ErrorCodeForbidden, message)
}

// ForbiddenWithCode writes a 403 response with a stable machine-readable code.
func ForbiddenWithCode(c *gin.Context, errorCode ErrorCode, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:      http.StatusForbidden,
		Message:   message,
		ErrorCode: errorCode,
	})
}

// NotFound writes a 404 response.
func NotFound(c *gin.Context, message string) {
	NotFoundWithCode(c, ErrorCodeNotFound, message)
}

// NotFoundWithCode writes a 404 response with a stable machine-readable code.
func NotFoundWithCode(c *gin.Context, errorCode ErrorCode, message string) {
	c.JSON(http.StatusNotFound, Response{
		Code:      http.StatusNotFound,
		Message:   message,
		ErrorCode: errorCode,
	})
}

// InternalServerError writes a generic 500 response and logs details.
func InternalServerError(c *gin.Context, detail string) {
	InternalServerErrorWithCode(c, ErrorCodeInternalServerError, detail)
}

// InternalServerErrorWithCode writes a generic 500 response and logs details.
func InternalServerErrorWithCode(c *gin.Context, errorCode ErrorCode, detail string) {
	if detail != "" && logger.Logger != nil {
		logger.Error("internal server error", logger.String("detail", detail))
	}
	c.JSON(http.StatusInternalServerError, Response{
		Code:      http.StatusInternalServerError,
		Message:   "internal server error",
		ErrorCode: errorCode,
	})
}

func httpStatusFromCode(code int) int {
	if code >= http.StatusBadRequest && code <= 599 {
		return code
	}
	return http.StatusInternalServerError
}

// PageSuccess writes a successful paginated response.
func PageSuccess(c *gin.Context, data any, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"list":      data,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
