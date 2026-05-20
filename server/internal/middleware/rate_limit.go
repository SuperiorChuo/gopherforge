package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// 时间窗口（秒）
	Window time.Duration
	// 最大请求数
	MaxRequests int
	// 键前缀
	KeyPrefix string
}

// DefaultRateLimitConfig 默认限流配置
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 60,
		KeyPrefix:   "rate_limit",
	}
}

// RateLimit 限流中间件
func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP
		clientIP := c.ClientIP()
		key := fmt.Sprintf("%s:%s", config.KeyPrefix, clientIP)

		ctx := context.Background()

		// 获取当前计数
		count, err := redis.Client.Get(ctx, key).Int()
		if err != nil && err.Error() != "redis: nil" {
			logger.Error("限流检查失败", logger.Err(err))
			c.Next()
			return
		}

		// 如果超过限制
		if count >= config.MaxRequests {
			response.Error(c, 429, "too many requests")
			c.Abort()
			return
		}

		// 增加计数
		pipe := redis.Client.Pipeline()
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, config.Window)
		_, err = pipe.Exec(ctx)
		if err != nil {
			logger.Error("限流计数增加失败", logger.Err(err))
		}

		c.Next()
	}
}
