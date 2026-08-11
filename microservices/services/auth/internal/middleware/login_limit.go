package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	redisstore "github.com/go-admin-kit/services/auth/internal/pkg/redis"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
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
	// IPShield* 是 IP 级失败护盾参数（独立于账号锁定，跨账号按 IP 计数）。
	IPShieldWindow       time.Duration
	IPShieldMaxFailures  int
	IPShieldBlockMinutes time.Duration
}

func LoginLimitConfigFromApp() LoginLimitConfig {
	return LoginLimitConfigFromPolicy(runtimeconfig.SecurityPolicyFromConfig())
}

func LoginLimitConfigFromPolicy(policy runtimeconfig.SecurityPolicy) LoginLimitConfig {
	window := time.Duration(policy.LoginLimitWindowMinutes) * time.Minute
	lockDuration := time.Duration(policy.LoginLimitLockMinutes) * time.Minute
	return LoginLimitConfig{
		Window:               window,
		MaxFailures:          policy.LoginLimitMaxFailures,
		LockDuration:         lockDuration,
		KeyPrefix:            "login_limit",
		IPShieldWindow:       time.Duration(policy.LoginIPShieldWindowMinutes) * time.Minute,
		IPShieldMaxFailures:  policy.LoginIPShieldMaxFailures,
		IPShieldBlockMinutes: time.Duration(policy.LoginIPShieldBlockMinutes) * time.Minute,
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

// RecordLoginFailureContext records a failed login: the per-account limiter
// keys on identifier ("user:ip"), and the IP shield keys on the real client IP
// passed separately — identifier slicing cannot recover IPv6 addresses.
func RecordLoginFailureContext(ctx context.Context, identifier, ip string, config LoginLimitConfig) {
	NewLoginLimiter().RecordFailureContext(ctx, identifier, config)
	RecordIPFailureContext(ctx, ip, IPFailureConfigFromLoginLimit(config))
}

// IPFailureConfigFromLoginLimit derives the IP-wide shield parameters from the
// runtime policy (falling back to hardcoded defaults when policy fields are 0).
func IPFailureConfigFromLoginLimit(config LoginLimitConfig) IPFailureConfig {
	cfg := DefaultIPFailureConfig()
	if config.IPShieldWindow > 0 {
		cfg.Window = config.IPShieldWindow
	}
	if config.IPShieldMaxFailures > 0 {
		cfg.MaxFailures = int64(config.IPShieldMaxFailures)
	}
	if config.IPShieldBlockMinutes > 0 {
		cfg.BlockMinutes = config.IPShieldBlockMinutes
	}
	return cfg
}

// IPFailureConfig controls the coarse IP-wide failure shield: a single IP
// failing logins across many usernames gets a temporary block (10 min) after
// the threshold, independent of per-account locks.
type IPFailureConfig struct {
	Window       time.Duration
	MaxFailures  int64
	BlockMinutes time.Duration
	KeyPrefix    string
}

// loginIPShieldPrefix is the shared Redis key prefix for the IP failure shield.
// Both the counter/block writers and IsIPBlockedContext read it — never
// hardcode the prefix in the lookup side (silent divergence otherwise).
const loginIPShieldPrefix = "login_ip_fail"

func DefaultIPFailureConfig() IPFailureConfig {
	return IPFailureConfig{
		Window:       10 * time.Minute,
		MaxFailures:  30,
		BlockMinutes: 10 * time.Minute,
		KeyPrefix:    loginIPShieldPrefix,
	}
}

// RecordIPFailureContext bumps a per-IP failure counter and blocks the IP once
// the threshold is crossed. Best-effort: redis unavailable is ignored.
func RecordIPFailureContext(ctx context.Context, ip string, cfg IPFailureConfig) {
	client := redisstore.Client
	if client == nil {
		return
	}
	if ip == "" || ip == ":" {
		return
	}
	countKey := cfg.KeyPrefix + ":" + ip
	count, err := client.Incr(ctx, countKey).Result()
	if err != nil {
		return
	}
	if count == 1 {
		_ = client.Expire(ctx, countKey, cfg.Window).Err()
	}
	if count >= cfg.MaxFailures {
		_ = client.Set(ctx, cfg.KeyPrefix+":block:"+ip, "1", cfg.BlockMinutes).Err()
	}
}

// IsIPBlockedContext reports whether an IP is under the coarse failure shield.
func IsIPBlockedContext(ctx context.Context, ip string) bool {
	client := redisstore.Client
	if client == nil {
		return false
	}
	blocked, err := client.Get(ctx, loginIPShieldPrefix+":block:"+ip).Result()
	return err == nil && blocked == "1"
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
