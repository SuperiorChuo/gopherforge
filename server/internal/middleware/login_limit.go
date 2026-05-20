package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

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

func IsLoginLocked(identifier string, config LoginLimitConfig) (bool, time.Duration) {
	return IsLoginLockedContext(context.Background(), identifier, config)
}

func IsLoginLockedContext(ctx context.Context, identifier string, config LoginLimitConfig) (bool, time.Duration) {
	if redis.Client == nil {
		return false, 0
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
	lockKey := fmt.Sprintf("%s:lock", key)
	locked, err := redis.Client.Get(ctx, lockKey).Result()
	if err == nil && locked == "1" {
		ttl, _ := redis.Client.TTL(ctx, lockKey).Result()
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
	return func(c *gin.Context) {
		identifier := c.ClientIP()
		if username := c.PostForm("username"); username != "" {
			identifier = username
		}

		key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
		ctx := c.Request.Context()

		locked, err := redis.Client.Get(ctx, fmt.Sprintf("%s:lock", key)).Result()
		if err == nil && locked == "1" {
			ttl, _ := redis.Client.TTL(ctx, fmt.Sprintf("%s:lock", key)).Result()
			response.Error(c, 429, fmt.Sprintf("account is locked, please try again after %d seconds", int(ttl.Seconds())))
			c.Abort()
			return
		}

		c.Next()
	}
}

// RecordLoginFailure records a failed login attempt.
func RecordLoginFailure(identifier string, config LoginLimitConfig) {
	RecordLoginFailureContext(context.Background(), identifier, config)
}

func RecordLoginFailureContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	if redis.Client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)

	failures, err := redis.Client.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	redis.Client.Expire(ctx, key, config.Window)

	if failures >= int64(config.MaxFailures) {
		lockKey := fmt.Sprintf("%s:lock", key)
		redis.Client.Set(ctx, lockKey, "1", config.LockDuration)
		logger.Warn("account locked after repeated login failures",
			logger.String("identifier", identifier),
			logger.Int64("failures", failures),
		)
	}
}

// ClearLoginLimit clears login throttling after a successful login.
func ClearLoginLimit(identifier string, config LoginLimitConfig) {
	ClearLoginLimitContext(context.Background(), identifier, config)
}

func ClearLoginLimitContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	if redis.Client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)

	redis.Client.Del(ctx, key)
	redis.Client.Del(ctx, fmt.Sprintf("%s:lock", key))
}
