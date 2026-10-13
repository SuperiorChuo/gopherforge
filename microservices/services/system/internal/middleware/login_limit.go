package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
)

// LoginLimitRedisClient 是 LoginLimiter 使用的 Redis 命令子集。
type LoginLimitRedisClient interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	TTL(ctx context.Context, key string) *goredis.DurationCmd
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd
	Del(ctx context.Context, keys ...string) *goredis.IntCmd
}

// LoginLimiter 跟踪登录失败与账号锁定。
type LoginLimiter struct {
	client LoginLimitRedisClient
}

// NewLoginLimiter 创建基于包级 Redis 客户端的限流器。
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{}
}

// NewLoginLimiterWithClient 创建基于指定 Redis 客户端的限流器。
func NewLoginLimiterWithClient(client LoginLimitRedisClient) *LoginLimiter {
	return &LoginLimiter{client: client}
}

// LoginLimitConfig 控制登录失败的节流策略。
type LoginLimitConfig struct {
	Window       time.Duration
	MaxFailures  int
	LockDuration time.Duration
	KeyPrefix    string
}

func LoginLimitConfigFromApp() LoginLimitConfig {
	return LoginLimitConfigFromPolicy(runtimeconfig.SecurityPolicyFromConfig())
}

func LoginLimitConfigFromPolicy(policy runtimeconfig.SecurityPolicy) LoginLimitConfig {
	window := time.Duration(policy.LoginLimitWindowMinutes) * time.Minute
	lockDuration := time.Duration(policy.LoginLimitLockMinutes) * time.Minute
	return LoginLimitConfig{
		Window:       window,
		MaxFailures:  policy.LoginLimitMaxFailures,
		LockDuration: lockDuration,
		KeyPrefix:    "login_limit",
	}
}

func LoginLimitEnabled() bool {
	return runtimeconfig.SecurityPolicyFromConfig().LoginLimitEnabled
}

func LoginIdentifier(username, ip string) string {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return ip
	}
	return fmt.Sprintf("%s:%s", username, ip)
}

func IsLoginLockedContext(ctx context.Context, identifier string, config LoginLimitConfig) (bool, time.Duration) {
	return NewLoginLimiter().IsLockedContext(ctx, identifier, config)
}

// IsLockedContext 报告该标识是否当前处于锁定状态。
func (l *LoginLimiter) IsLockedContext(ctx context.Context, identifier string, config LoginLimitConfig) (bool, time.Duration) {
	client := l.redisClient()
	if client == nil {
		return false, 0
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
	lockKey := fmt.Sprintf("%s:lock", key)
	locked, err := client.Get(ctx, lockKey).Result()
	if err == nil && locked == "1" {
		ttl, _ := client.TTL(ctx, lockKey).Result()
		return true, ttl
	}
	return false, 0
}

// DefaultLoginLimitConfig 返回登录节流的默认配置。
func DefaultLoginLimitConfig() LoginLimitConfig {
	return LoginLimitConfig{
		Window:       time.Minute * 15,
		MaxFailures:  5,
		LockDuration: time.Minute * 30,
		KeyPrefix:    "login_limit",
	}
}

// CheckLoginLimit 在标识被锁定时拦截登录请求。
func CheckLoginLimit(config LoginLimitConfig) gin.HandlerFunc {
	return NewLoginLimiter().Check(config)
}

// Check 在标识被锁定时拦截登录请求。
func (l *LoginLimiter) Check(config LoginLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := c.ClientIP()
		if username := c.PostForm("username"); username != "" {
			identifier = username
		}

		key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
		ctx := c.Request.Context()
		client := l.redisClient()
		if client == nil {
			c.Next()
			return
		}

		locked, err := client.Get(ctx, fmt.Sprintf("%s:lock", key)).Result()
		if err == nil && locked == "1" {
			ttl, _ := client.TTL(ctx, fmt.Sprintf("%s:lock", key)).Result()
			response.ErrorWithCode(c, 429, response.ErrorCodeLoginLocked, fmt.Sprintf("account is locked, please try again after %d seconds", int(ttl.Seconds())))
			c.Abort()
			return
		}

		c.Next()
	}
}

func RecordLoginFailureContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	NewLoginLimiter().RecordFailureContext(ctx, identifier, config)
}

// RecordFailureContext 记录一次失败的登录尝试。
func (l *LoginLimiter) RecordFailureContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	client := l.redisClient()
	if client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)

	failures, err := client.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	client.Expire(ctx, key, config.Window)

	if failures >= int64(config.MaxFailures) {
		lockKey := fmt.Sprintf("%s:lock", key)
		client.Set(ctx, lockKey, "1", config.LockDuration)
		logger.Warn("account locked after repeated login failures",
			logger.String("identifier", identifier),
			logger.Int64("failures", failures),
		)
	}
}

func ClearLoginLimitContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	NewLoginLimiter().ClearContext(ctx, identifier, config)
}

// ClearContext 在登录成功后清除登录节流状态。
func (l *LoginLimiter) ClearContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	client := l.redisClient()
	if client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)

	client.Del(ctx, key)
	client.Del(ctx, fmt.Sprintf("%s:lock", key))
}

func (l *LoginLimiter) redisClient() LoginLimitRedisClient {
	if l != nil && l.client != nil {
		return l.client
	}
	return redisstore.Client
}
