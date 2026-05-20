package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

type RateLimitConfig struct {
	Window      time.Duration
	MaxRequests int
	KeyPrefix   string
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 60,
		KeyPrefix:   "rate_limit",
	}
}

func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := fmt.Sprintf("%s:%s", config.KeyPrefix, clientIP)
		ctx := c.Request.Context()

		count, err := redis.Client.Incr(ctx, key).Result()
		if err != nil {
			logger.Error("rate limit check failed", logger.Err(err))
			c.Next()
			return
		}
		if count == 1 {
			if err := redis.Client.Expire(ctx, key, config.Window).Err(); err != nil {
				logger.Error("rate limit expire failed", logger.Err(err))
			}
		}
		if count > int64(config.MaxRequests) {
			response.Error(c, 429, "too many requests")
			c.Abort()
			return
		}

		c.Next()
	}
}
