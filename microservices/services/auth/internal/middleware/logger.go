package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/pkg/logger"
)

// RequestLogger writes structured request logs.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := requestLogPath(c.Request.URL)

		c.Next()

		latency := time.Since(start)

		clientIP := c.ClientIP()

		method := c.Request.Method

		statusCode := c.Writer.Status()

		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

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

func requestLogPath(u *url.URL) string {
	if u == nil {
		return ""
	}
	path := u.Path
	raw := sanitizeRawQuery(u.RawQuery)
	if raw == "" {
		return path
	}
	return path + "?" + raw
}

func sanitizeRawQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return sanitizeRawQueryPairs(raw)
	}
	for key := range values {
		if isSensitiveQueryKey(key) {
			values[key] = []string{"***"}
		}
	}
	return values.Encode()
}

func sanitizeRawQueryPairs(raw string) string {
	parts := strings.Split(raw, "&")
	for i, part := range parts {
		key, _, hasValue := strings.Cut(part, "=")
		if isSensitiveQueryKey(key) {
			if hasValue {
				parts[i] = key + "=***"
			} else {
				parts[i] = key
			}
		}
	}
	return strings.Join(parts, "&")
}

func isSensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "ticket", "token", "access_token", "refresh_token", "secret", "password":
		return true
	default:
		return false
	}
}
