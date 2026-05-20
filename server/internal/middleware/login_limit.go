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

// LoginLimitConfig 登录限制配置
type LoginLimitConfig struct {
	// 时间窗口（秒）
	Window time.Duration
	// 最大失败次数
	MaxFailures int
	// 锁定时间（秒）
	LockDuration time.Duration
	// 键前缀
	KeyPrefix string
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
	if redis.Client == nil {
		return false, 0
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
	lockKey := fmt.Sprintf("%s:lock", key)
	ctx := context.Background()
	locked, err := redis.Client.Get(ctx, lockKey).Result()
	if err == nil && locked == "1" {
		ttl, _ := redis.Client.TTL(ctx, lockKey).Result()
		return true, ttl
	}
	return false, 0
}

// DefaultLoginLimitConfig 默认登录限制配置
func DefaultLoginLimitConfig() LoginLimitConfig {
	return LoginLimitConfig{
		Window:       time.Minute * 15,
		MaxFailures:  5,
		LockDuration: time.Minute * 30,
		KeyPrefix:    "login_limit",
	}
}

// CheckLoginLimit 检查登录限制（用于登录接口）
func CheckLoginLimit(config LoginLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP或用户名
		identifier := c.ClientIP()
		if username := c.PostForm("username"); username != "" {
			identifier = username
		}

		key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
		ctx := context.Background()

		// 检查是否被锁定
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

// RecordLoginFailure 记录登录失败
func RecordLoginFailure(identifier string, config LoginLimitConfig) {
	if redis.Client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
	ctx := context.Background()

	// 增加失败次数
	failures, _ := redis.Client.Incr(ctx, key).Result()
	redis.Client.Expire(ctx, key, config.Window)

	// 如果超过最大失败次数，锁定账户
	if failures >= int64(config.MaxFailures) {
		lockKey := fmt.Sprintf("%s:lock", key)
		redis.Client.Set(ctx, lockKey, "1", config.LockDuration)
		logger.Warn("账户因登录失败次数过多已被锁定",
			logger.String("标识", identifier),
			logger.Int64("失败次数", failures),
		)
	}
}

// ClearLoginLimit 清除登录限制（登录成功后调用）
func ClearLoginLimit(identifier string, config LoginLimitConfig) {
	if redis.Client == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", config.KeyPrefix, identifier)
	ctx := context.Background()

	redis.Client.Del(ctx, key)
	redis.Client.Del(ctx, fmt.Sprintf("%s:lock", key))
}
