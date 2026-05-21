package auth

import (
	"strings"

	"github.com/go-admin-kit/server/internal/config"
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

func SecureConsoleCookie() bool {
	return strings.EqualFold(config.Cfg.App.Env, "production") || config.Cfg.Security.Headers.HSTS
}
