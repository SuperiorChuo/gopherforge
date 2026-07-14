package auth

import (
	"context"
	"strings"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
)

type ConsoleSecurityPolicy struct {
	SessionTTLMinutes       int
	LoginMaxAttemptsPerHour int
	LockoutMinutes          int
}

func DefaultConsoleSecurityPolicy() ConsoleSecurityPolicy {
	sessionTTL := config.Cfg.JWT.AccessTokenExpire
	if sessionTTL <= 0 {
		sessionTTL = 480
	}
	maxAttempts := config.Cfg.Security.LoginLimit.MaxFailures
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	lockoutMinutes := config.Cfg.Security.LoginLimit.LockMinutes
	if lockoutMinutes <= 0 {
		lockoutMinutes = 15
	}
	return ConsoleSecurityPolicy{
		SessionTTLMinutes:       sessionTTL,
		LoginMaxAttemptsPerHour: maxAttempts,
		LockoutMinutes:          lockoutMinutes,
	}
}

func DefaultConsoleSecurityPolicyContext(ctx context.Context) ConsoleSecurityPolicy {
	policy := runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(ctx)
	return consoleSecurityPolicy(policy)
}

func consoleSecurityPolicy(policy runtimeconfig.SecurityPolicy) ConsoleSecurityPolicy {
	sessionTTL := config.Cfg.JWT.AccessTokenExpire
	if sessionTTL <= 0 {
		sessionTTL = 480
	}
	maxAttempts := policy.LoginLimitMaxFailures
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	lockoutMinutes := policy.LoginLimitLockMinutes
	if lockoutMinutes <= 0 {
		lockoutMinutes = 15
	}
	return ConsoleSecurityPolicy{
		SessionTTLMinutes:       sessionTTL,
		LoginMaxAttemptsPerHour: maxAttempts,
		LockoutMinutes:          lockoutMinutes,
	}
}

func SecureConsoleCookie() bool {
	return strings.EqualFold(config.Cfg.App.Env, "production") || config.Cfg.Security.Headers.HSTS
}
