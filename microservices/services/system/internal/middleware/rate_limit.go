package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
)

// RateLimitRedisClient 是 RateLimiter 使用的 Redis 命令子集。
type RateLimitRedisClient interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
	ExpireNX(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
}

// RateLimiter 强制实施请求速率限制。
type RateLimiter struct {
	client RateLimitRedisClient
}

// NewRateLimiter 创建基于包级 Redis 客户端的限流器。
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{}
}

// NewRateLimiterWithClient 创建基于指定 Redis 客户端的限流器。
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

// Middleware 返回使用限流器 Redis 客户端的 Gin 中间件。
func (l *RateLimiter) Middleware(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if sharedmw.IsHealthProbePath(c.Request.URL.Path) {
			c.Next()
			return
		}
		l.apply(c, config)
	}
}

func (l *RateLimiter) DynamicMiddleware(reader runtimeconfig.SecurityPolicyReader) gin.HandlerFunc {
	if reader == nil {
		reader = runtimeconfig.DefaultSecurityPolicyReader()
	}
	return func(c *gin.Context) {
		if sharedmw.IsHealthProbePath(c.Request.URL.Path) {
			c.Next()
			return
		}
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
	// 使用 ExpireNX 而非 count==1+Expire：如果首个请求的 Expire 丢失
	// （在 INCR 与 EXPIRE 之间崩溃，或调用失败），键会永久存在，
	// 从而永久限流该客户端。
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
