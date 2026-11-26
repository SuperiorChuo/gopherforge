package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
)

type (
	LoginLimitRedisClient = sharedmw.LoginLimitRedisClient
	LoginLimiter          = sharedmw.LoginLimiter
	LoginLimitConfig      = sharedmw.LoginLimitConfig
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

func RecordLoginFailureContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	sharedmw.RecordLoginFailureContext(ctx, identifier, config)
}

func ClearLoginLimitContext(ctx context.Context, identifier string, config LoginLimitConfig) {
	sharedmw.ClearLoginLimitContext(ctx, identifier, config)
}
