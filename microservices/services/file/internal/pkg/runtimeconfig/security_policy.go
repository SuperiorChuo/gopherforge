package runtimeconfig

import (
	"time"

	"github.com/go-admin-kit/services/file/internal/config"
	systemdao "github.com/go-admin-kit/services/file/internal/dao/system"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
)

const SecurityPolicySettingKey = sharedruntimeconfig.SecurityPolicySettingKey

var ErrStoreUnavailable = sharedruntimeconfig.ErrStoreUnavailable

type (
	SecurityPolicy             = sharedruntimeconfig.SecurityPolicy
	SecurityPolicyReader       = sharedruntimeconfig.SecurityPolicyReader
	SecurityPolicyInvalidator  = sharedruntimeconfig.SecurityPolicyInvalidator
	SecurityPolicyStore        = sharedruntimeconfig.SecurityPolicyStore
	CachedSecurityPolicyReader = sharedruntimeconfig.CachedSecurityPolicyReader
)

var defaultPolicyHolder = sharedruntimeconfig.NewDefaultSecurityPolicyHolder(
	SecurityPolicyFromConfig,
	func() SecurityPolicyStore { return systemdao.NewSettingDAO(nil) },
)

func NewCachedSecurityPolicyReader(store SecurityPolicyStore, ttl time.Duration) *CachedSecurityPolicyReader {
	return sharedruntimeconfig.NewCachedSecurityPolicyReader(store, ttl, SecurityPolicyFromConfig)
}

func DefaultSecurityPolicyReader() *CachedSecurityPolicyReader {
	return defaultPolicyHolder.Reader()
}

func SetSecurityPolicyStore(store SecurityPolicyStore) func() {
	return defaultPolicyHolder.SetStore(store)
}

func SecurityPolicyFromConfig() SecurityPolicy {
	loginLimit := config.Cfg.Security.LoginLimit
	rateLimit := config.Cfg.Security.RateLimit
	return SecurityPolicy{
		PasswordMaxAgeDays:         config.Cfg.Security.EffectivePasswordMaxAgeDays(),
		PasswordHistoryCount:       config.Cfg.Security.EffectivePasswordHistoryCount(),
		LoginLimitEnabled:          loginLimit.Enabled,
		LoginLimitMaxFailures:      sharedruntimeconfig.PositiveOrDefault(loginLimit.MaxFailures, 5),
		LoginLimitWindowMinutes:    sharedruntimeconfig.PositiveOrDefault(loginLimit.WindowMinutes, 15),
		LoginLimitLockMinutes:      sharedruntimeconfig.PositiveOrDefault(loginLimit.LockMinutes, 30),
		LoginIPShieldMaxFailures:   30,
		LoginIPShieldWindowMinutes: 10,
		LoginIPShieldBlockMinutes:  10,
		LoginAlertEnabled:          true,
		RateLimitEnabled:           rateLimit.Enabled,
		RateLimitWindowSeconds:     sharedruntimeconfig.PositiveOrDefault(rateLimit.WindowSeconds, 1),
		RateLimitMaxRequests:       sharedruntimeconfig.PositiveOrDefault(rateLimit.MaxRequests, 100),
	}
}
