package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
)

// RequestLogger writes structured request logs.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)

		clientIP := c.ClientIP()

		method := c.Request.Method

		statusCode := c.Writer.Status()

		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("http request",
			logger.String("request_id", GetRequestID(c)),
			logger.String("method", method),
			logger.String("path", path),
			logger.Int("status_code", statusCode),
			logger.String("IP", clientIP),
			logger.Duration("latency", latency),
			logger.String("error", errMsg),
		)
	}
}
