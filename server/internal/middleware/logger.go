package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
)

// RequestLogger 请求日志中间件
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 结束时间
		latency := time.Since(start)

		// 客户端IP
		clientIP := c.ClientIP()

		// 方法
		method := c.Request.Method

		// 状态码
		statusCode := c.Writer.Status()

		// 错误信息
		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		// 构建查询字符串
		if raw != "" {
			path = path + "?" + raw
		}

		// 记录日志
		logger.Info("HTTP 请求",
			logger.String("请求ID", GetRequestID(c)),
			logger.String("方法", method),
			logger.String("路径", path),
			logger.Int("状态码", statusCode),
			logger.String("IP", clientIP),
			logger.Duration("耗时", latency),
			logger.String("错误", errMsg),
		)
	}
}
