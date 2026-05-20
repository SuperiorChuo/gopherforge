package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/errors"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// ErrorHandler 错误处理中间件
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 检查是否有错误
		if len(c.Errors) > 0 {
			err := c.Errors.Last()

			// 处理应用错误
			if appErr, ok := err.Err.(*errors.AppError); ok {
				response.Error(c, appErr.Code, appErr.Message)
				c.Abort()
				return
			}

			// 处理其他错误
			logger.Error("请求错误", logger.Err(err.Err))
			response.InternalServerError(c, "内部服务器错误")
			c.Abort()
			return
		}

		// 处理 404
		if c.Writer.Status() == http.StatusNotFound {
			response.NotFound(c, "resource not found")
			c.Abort()
			return
		}
	}
}

// Recovery 恢复中间件（捕获 panic）
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		logger.Error("Panic 已恢复", logger.Any("错误", recovered))
		response.InternalServerError(c, "内部服务器错误")
		c.Abort()
	})
}
