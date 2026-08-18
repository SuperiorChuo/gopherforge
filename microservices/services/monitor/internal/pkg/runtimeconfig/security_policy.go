package runtimeconfig

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/config"
	systemdao "github.com/go-admin-kit/services/monitor/internal/dao/system"
	"github.com/go-admin-kit/services/shared/pkg/database"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
)

const SecurityPolicySettingKey = "security.policy"

var ErrStoreUnavailable = errors.New("runtime config store unavailable")

type SecurityPolicy struct {
	PasswordMaxAgeDays      int
	PasswordHistoryCount    int
	LoginLimitEnabled       bool
	LoginLimitMaxFailures   int
	LoginLimitWindowMinutes int
	LoginLimitLockMinutes   int
	RateLimitEnabled        bool
	RateLimitWindowSeconds  int
	RateLimitMaxRequests    int
}

type SecurityPolicyReader interface {
	SecurityPolicy(ctx context.Context) SecurityPolicy
}

type SecurityPolicyInvalidator interface {
	Refresh(ctx context.Context) error
}

type SecurityPolicyStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

type CachedSecurityPolicyReader struct {
	reader *sharedruntimeconfig.CachedSettingReader[SecurityPolicy]
}

func NewCachedSecurityPolicyReader(store SecurityPolicyStore, ttl time.Duration) *CachedSecurityPolicyReader {
	return &CachedSecurityPolicyReader{reader: sharedruntimeconfig.NewCachedSettingReader(
		store, SecurityPolicySettingKey, ttl, SecurityPolicyFromConfig, applySecurityPolicySetting,
	)}
}

var (
	defaultSecurityPolicyOnce   sync.Once
	defaultSecurityPolicyReader *CachedSecurityPolicyReader
)

func DefaultSecurityPolicyReader() *CachedSecurityPolicyReader {
	defaultSecurityPolicyOnce.Do(func() {
		defaultSecurityPolicyReader = NewCachedSecurityPolicyReader(defaultSecurityPolicyStore{}, 30*time.Second)
	})
	return defaultSecurityPolicyReader
}

var (
	securityPolicyStoreMu sync.RWMutex
	securityPolicyStore   SecurityPolicyStore
)

// SetSecurityPolicyStore installs the store behind DefaultSecurityPolicyReader
// and returns a restore function. The default reader resolves the store per
// lookup, so wiring only needs to happen before the first request is served.
func SetSecurityPolicyStore(store SecurityPolicyStore) func() {
	securityPolicyStoreMu.Lock()
	previous := securityPolicyStore
	securityPolicyStore = store
	securityPolicyStoreMu.Unlock()

	return func() {
		securityPolicyStoreMu.Lock()
		securityPolicyStore = previous
		securityPolicyStoreMu.Unlock()
	}
}

type defaultSecurityPolicyStore struct{}

func (defaultSecurityPolicyStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	securityPolicyStoreMu.RLock()
	store := securityPolicyStore
	securityPolicyStoreMu.RUnlock()
	if store != nil {
		return store.GetByKeyContext(ctx, key)
	}
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	return systemdao.NewSettingDAO(nil).GetByKeyContext(ctx, key)
}

func (r *CachedSecurityPolicyReader) SecurityPolicy(ctx context.Context) SecurityPolicy {
	if r == nil || r.reader == nil {
		return SecurityPolicyFromConfig()
	}
	return r.reader.Value(ctx)
}

func (r *CachedSecurityPolicyReader) Refresh(ctx context.Context) error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Refresh(ctx)
}

func SecurityPolicyFromConfig() SecurityPolicy {
	loginLimit := config.Cfg.Security.LoginLimit
	rateLimit := config.Cfg.Security.RateLimit
	return SecurityPolicy{
		PasswordMaxAgeDays:      config.Cfg.Security.EffectivePasswordMaxAgeDays(),
		PasswordHistoryCount:    config.Cfg.Security.EffectivePasswordHistoryCount(),
		LoginLimitEnabled:       loginLimit.Enabled,
		LoginLimitMaxFailures:   sharedruntimeconfig.PositiveOrDefault(loginLimit.MaxFailures, 5),
		LoginLimitWindowMinutes: sharedruntimeconfig.PositiveOrDefault(loginLimit.WindowMinutes, 15),
		LoginLimitLockMinutes:   sharedruntimeconfig.PositiveOrDefault(loginLimit.LockMinutes, 30),
		RateLimitEnabled:        rateLimit.Enabled,
		RateLimitWindowSeconds:  sharedruntimeconfig.PositiveOrDefault(rateLimit.WindowSeconds, 1),
		RateLimitMaxRequests:    sharedruntimeconfig.PositiveOrDefault(rateLimit.MaxRequests, 100),
	}
}

func applySecurityPolicySetting(policy SecurityPolicy, value map[string]any) SecurityPolicy {
	if value == nil {
		return policy
	}
	policy.PasswordMaxAgeDays = sharedruntimeconfig.NonNegativeSetting(value, "password_max_age_days", policy.PasswordMaxAgeDays)
	policy.PasswordHistoryCount = sharedruntimeconfig.NonNegativeSetting(value, "password_history_count", policy.PasswordHistoryCount)
	policy.LoginLimitMaxFailures = sharedruntimeconfig.PositiveSetting(value, "login_limit_max_failures", policy.LoginLimitMaxFailures)
	policy.LoginLimitWindowMinutes = sharedruntimeconfig.PositiveSetting(value, "login_limit_window_minutes", policy.LoginLimitWindowMinutes)
	policy.LoginLimitLockMinutes = sharedruntimeconfig.PositiveSetting(value, "login_limit_lock_minutes", policy.LoginLimitLockMinutes)
	if rps, ok := sharedruntimeconfig.PositiveInt(value["rate_limit_rps"]); ok {
		policy.RateLimitWindowSeconds = 1
		policy.RateLimitMaxRequests = rps
	}
	return policy
}
