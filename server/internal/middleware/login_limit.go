package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
	goredis "github.com/redis/go-redis/v9"
)

// LoginLimitRedisClient is the Redis command subset used by LoginLimiter.
type LoginLimitRedisClient interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	TTL(ctx context.Context, key string) *goredis.DurationCmd
	Incr(ctx context.Context, key string) *goredis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd
	Del(ctx context.Context, keys ...string) *goredis.IntCmd
}

// LoginLimiter tracks login failures and lockouts.
type LoginLimiter struct {
	client LoginLimitRedisClient
}

// NewLoginLimiter creates a limiter backed by the package Redis client.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{}
}

// NewLoginLimiterWithClient creates a limiter backed by the provided Redis client.
func NewLoginLimiterWithClient(client LoginLimitRedisClient) *LoginLimiter {
	return &LoginLimiter{client: client}
}

// LoginLimitConfig controls login failure throttling.
type LoginLimitConfig struct {
	Window       time.Duration
	MaxFailures  int
	LockDuration time.Duration
	KeyPrefix    string
}

func LoginLimitConfigFromApp() LoginLimitConfig {
	cfg := config.Cfg.Security.LoginLimit
	window := time.Duration(cfg.WindowMinutes) * time.Minute
	if window <= 0 {
		window = 15 * time.Minute
	}
	lockDuration := time.Duration(cfg.LockMinutes) * time.Minute
	if lockDuration <= 0 {
		lockDuration = 30 * time.Minute
	}
	maxFailures := cfg.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}
	return LoginLimitConfig{
		Window:       window,
		MaxFailures:  maxFailures,
		LockDuration: lockDuration,
		KeyPrefix:    "login_limit",
	}
}

func LoginLimitEnabled() bool {
	return config.Cfg.Security.LoginLimit.Enabled
}

func LoginIdentifier(username, ip string) string {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return ip
	}
	return fmt.Sprintf("%s:%s", username, ip)
}

// Deprecated: use IsLoginLockedContext instead.
func IsLoginLocked(identifier string, config LoginLimitConfig) (bool, time.Duration) {
	return IsLoginLockedContext(context.Background(), identifier, config)
}

func IsLoginLockedContext(ctx context.Context, identifier string, config LoginLimitConfig) (bool, time.Duration) {
	return NewLoginLimiter().IsLockedContext(ctx, identifier, config)
}

// IsLockedContext reports whether the identifier is currently locked.
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

// DefaultLoginLimitConfig returns fallback login throttling settings.
func DefaultLoginLimitConfig() LoginLimitConfig {
	return LoginLimitConfig{
		Window:       time.Minute * 15,
		MaxFailures:  5,
		LockDuration: time.Minute * 30,
		KeyPrefix:    "login_limit",
	}
}

// CheckLoginLimit blocks login requests while an identifier is locked.
func CheckLoginLimit(config LoginLimitConfig) gin.HandlerFunc {
	return NewLoginLimiter().Check(config)
}

// Check blocks login requests while an identifier is locked.
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

// RecordLoginFailure records a failed login attempt.
// Deprecated: use RecordLoginFailureContext instead.
func RecordLoginFailure(identifier string, config LoginLimitConfig) {
	RecordLoginFailureContext(context.Background(), identifier, config)
}

func RecordLoginFailureContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	NewLoginLimiter().RecordFailureContext(ctx, identifier, config)
}

// RecordFailureContext records a failed login attempt.
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

// ClearLoginLimit clears login throttling after a successful login.
// Deprecated: use ClearLoginLimitContext instead.
func ClearLoginLimit(identifier string, config LoginLimitConfig) {
	ClearLoginLimitContext(context.Background(), identifier, config)
}

func ClearLoginLimitContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	NewLoginLimiter().ClearContext(ctx, identifier, config)
}

// ClearContext clears login throttling after a successful login.
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
