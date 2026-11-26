package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
)

type (
	LoginLimitRedisClient = sharedmw.LoginLimitRedisClient
	LoginLimiter          = sharedmw.LoginLimiter
	LoginLimitConfig      = sharedmw.LoginLimitConfig
	IPFailureConfig       = sharedmw.IPFailureConfig
)

func NewLoginLimiter() *LoginLimiter { return sharedmw.NewLoginLimiter() }

func NewLoginLimiterWithClient(client LoginLimitRedisClient) *LoginLimiter {
	return sharedmw.NewLoginLimiterWithClient(client)
}

func LoginLimitConfigFromApp() LoginLimitConfig {
	return LoginLimitConfigFromPolicy(runtimeconfig.SecurityPolicyFromConfig())
}

func LoginLimitConfigFromPolicy(policy runtimeconfig.SecurityPolicy) LoginLimitConfig {
	return sharedmw.LoginLimitConfigFromPolicy(policy)
}

func LoginLimitEnabled() bool {
	return runtimeconfig.SecurityPolicyFromConfig().LoginLimitEnabled
}

func LoginIdentifier(username, ip string) string { return sharedmw.LoginIdentifier(username, ip) }

func IsLoginLockedContext(ctx context.Context, identifier string, config LoginLimitConfig) (bool, time.Duration) {
	return sharedmw.IsLoginLockedContext(ctx, identifier, config)
}

func DefaultLoginLimitConfig() LoginLimitConfig { return sharedmw.DefaultLoginLimitConfig() }

func CheckLoginLimit(config LoginLimitConfig) gin.HandlerFunc { return sharedmw.CheckLoginLimit(config) }

func RecordLoginFailureContext(ctx context.Context, identifier, ip string, config LoginLimitConfig) {
	sharedmw.RecordLoginFailureWithIPContext(ctx, identifier, ip, config)
}

func IPFailureConfigFromLoginLimit(config LoginLimitConfig) IPFailureConfig {
	return sharedmw.IPFailureConfigFromLoginLimit(config)
}

func DefaultIPFailureConfig() IPFailureConfig { return sharedmw.DefaultIPFailureConfig() }

func RecordIPFailureContext(ctx context.Context, ip string, cfg IPFailureConfig) {
	sharedmw.RecordIPFailureContext(ctx, ip, cfg)
}

func IsIPBlockedContext(ctx context.Context, ip string) bool {
	return sharedmw.IsIPBlockedContext(ctx, ip)
}

func ClearLoginLimitContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	sharedmw.ClearLoginLimitContext(ctx, identifier, config)
}
