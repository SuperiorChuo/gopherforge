package runtimeconfig

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/go-admin-kit/services/audit/internal/config"
	systemdao "github.com/go-admin-kit/services/audit/internal/dao/system"
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
	// LoginAlertEnabled 控制「新设备 / 新 IP 登录」站内信提醒。
	LoginAlertEnabled      bool
	RateLimitEnabled       bool
	RateLimitWindowSeconds int
	RateLimitMaxRequests   int
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
		LoginLimitMaxFailures:   positiveOrDefault(loginLimit.MaxFailures, 5),
		LoginLimitWindowMinutes: positiveOrDefault(loginLimit.WindowMinutes, 15),
		LoginLimitLockMinutes:   positiveOrDefault(loginLimit.LockMinutes, 30),
		LoginAlertEnabled:       true,
		RateLimitEnabled:        rateLimit.Enabled,
		RateLimitWindowSeconds:  positiveOrDefault(rateLimit.WindowSeconds, 1),
		RateLimitMaxRequests:    positiveOrDefault(rateLimit.MaxRequests, 100),
	}
}

func applySecurityPolicySetting(policy SecurityPolicy, value map[string]any) SecurityPolicy {
	if value == nil {
		return policy
	}
	policy.PasswordMaxAgeDays = nonNegativeSetting(value, "password_max_age_days", policy.PasswordMaxAgeDays)
	policy.PasswordHistoryCount = nonNegativeSetting(value, "password_history_count", policy.PasswordHistoryCount)
	policy.LoginLimitMaxFailures = positiveSetting(value, "login_limit_max_failures", policy.LoginLimitMaxFailures)
	policy.LoginLimitWindowMinutes = positiveSetting(value, "login_limit_window_minutes", policy.LoginLimitWindowMinutes)
	policy.LoginLimitLockMinutes = positiveSetting(value, "login_limit_lock_minutes", policy.LoginLimitLockMinutes)
	if enabled, ok := boolSetting(value["login_alert_enabled"]); ok {
		policy.LoginAlertEnabled = enabled
	}
	if rps, ok := positiveInt(value["rate_limit_rps"]); ok {
		policy.RateLimitWindowSeconds = 1
		policy.RateLimitMaxRequests = rps
	}
	return policy
}

func boolSetting(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}

func nonNegativeSetting(value map[string]any, key string, fallback int) int {
	if got, ok := intSetting(value[key]); ok && got >= 0 {
		return got
	}
	return fallback
}

func positiveSetting(value map[string]any, key string, fallback int) int {
	if got, ok := positiveInt(value[key]); ok {
		return got
	}
	return fallback
}

func positiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt(value any) (int, bool) {
	got, ok := intSetting(value)
	return got, ok && got > 0
}

func intSetting(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		if uint64(v) > uint64(math.MaxInt) {
			return 0, false
		}
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		if v > uint64(math.MaxInt) {
			return 0, false
		}
		return int(v), true
	case float64:
		if math.Trunc(v) != v || v > float64(math.MaxInt) || v < float64(math.MinInt) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}
