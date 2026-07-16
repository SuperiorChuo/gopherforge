package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/errors"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// ErrorHandler converts Gin errors into API responses.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()

			if appErr, ok := err.Err.(*errors.AppError); ok {
				response.Error(c, appErr.Code, appErr.Message)
				c.Abort()
				return
			}

			logger.Error("request error", logger.Err(err.Err))
			response.InternalServerError(c, "internal server error")
			c.Abort()
			return
		}

		if c.Writer.Status() == http.StatusNotFound {
			response.NotFound(c, "resource not found")
			c.Abort()
			return
		}
	}
}

// Recovery converts panics into API responses.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered", logger.Any("recovered", recovered))
		response.InternalServerError(c, "internal server error")
		c.Abort()
	})
}
