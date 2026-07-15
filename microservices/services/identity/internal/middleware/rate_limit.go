package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/pkg/logger"
	redisstore "github.com/go-admin-kit/services/identity/internal/pkg/redis"
	"github.com/go-admin-kit/services/identity/internal/pkg/response"
	"github.com/go-admin-kit/services/identity/internal/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
)

// RateLimitRedisClient is the Redis command subset used by RateLimiter.
type RateLimitRedisClient interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
	ExpireNX(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
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

func DynamicRateLimit(reader runtimeconfig.SecurityPolicyReader) gin.HandlerFunc {
	return NewRateLimiter().DynamicMiddleware(reader)
}

func RateLimitConfigFromPolicy(policy runtimeconfig.SecurityPolicy) RateLimitConfig {
	return RateLimitConfig{
		Window:      time.Duration(policy.RateLimitWindowSeconds) * time.Second,
		MaxRequests: policy.RateLimitMaxRequests,
		KeyPrefix:   "rate_limit",
	}
}

// Middleware returns a Gin middleware using the limiter's Redis client.
func (l *RateLimiter) Middleware(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		l.apply(c, config)
	}
}

func (l *RateLimiter) DynamicMiddleware(reader runtimeconfig.SecurityPolicyReader) gin.HandlerFunc {
	if reader == nil {
		reader = runtimeconfig.DefaultSecurityPolicyReader()
	}
	return func(c *gin.Context) {
		policy := reader.SecurityPolicy(c.Request.Context())
		if !policy.RateLimitEnabled {
			c.Next()
			return
		}
		l.apply(c, RateLimitConfigFromPolicy(policy))
	}
}

func (l *RateLimiter) apply(c *gin.Context, config RateLimitConfig) {
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
	// ExpireNX instead of count==1+Expire: if the first request's Expire is
	// lost (crash between INCR and EXPIRE, or a failed call), the key would
	// otherwise live forever and permanently rate-limit the client.
	if err := client.ExpireNX(ctx, key, config.Window).Err(); err != nil {
		logger.Error("rate limit expire failed", logger.Err(err))
	}
	if count > int64(config.MaxRequests) {
		response.ErrorWithCode(c, 429, response.ErrorCodeRateLimited, "too many requests")
		c.Abort()
		return
	}

	c.Next()
}

func (l *RateLimiter) redisClient() RateLimitRedisClient {
	if l != nil && l.client != nil {
		return l.client
	}
	return redisstore.Client
}
