package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
	goredis "github.com/redis/go-redis/v9"
)

// RateLimitRedisClient is the Redis command subset used by RateLimiter.
type RateLimitRedisClient interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
}

// RateLimiter enforces request rate limits.
type RateLimiter struct {
	client RateLimitRedisClient
}

// NewRateLimiter creates a limiter backed by the package Redis client.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{}
}

// NewRateLimiterWithClient creates a limiter backed by the provided Redis client.
func NewRateLimiterWithClient(client RateLimitRedisClient) *RateLimiter {
	return &RateLimiter{client: client}
}

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
	return NewRateLimiter().Middleware(config)
}

// Middleware returns a Gin middleware using the limiter's Redis client.
func (l *RateLimiter) Middleware(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := fmt.Sprintf("%s:%s", config.KeyPrefix, clientIP)
		ctx := c.Request.Context()
		client := l.redisClient()
		if client == nil {
			c.Next()
			return
		}

		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			logger.Error("rate limit check failed", logger.Err(err))
			c.Next()
			return
		}
		if count == 1 {
			if err := client.Expire(ctx, key, config.Window).Err(); err != nil {
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

func (l *RateLimiter) redisClient() RateLimitRedisClient {
	if l != nil && l.client != nil {
		return l.client
	}
	return redisstore.Client
}
