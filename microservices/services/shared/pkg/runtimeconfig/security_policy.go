package runtimeconfig

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/database"
	model "github.com/go-admin-kit/services/shared/pkg/model"
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
	// LoginIPShield* 是 IP 级失败护盾参数（独立于账号锁定）。
	LoginIPShieldMaxFailures   int
	LoginIPShieldWindowMinutes int
	LoginIPShieldBlockMinutes  int
	// LoginAlertEnabled 控制 audit 侧「新设备/新 IP 登录」站内信提醒。
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
	reader   *CachedSettingReader[SecurityPolicy]
	defaults func() SecurityPolicy
}

func NewCachedSecurityPolicyReader(
	store SecurityPolicyStore,
	ttl time.Duration,
	defaults func() SecurityPolicy,
) *CachedSecurityPolicyReader {
	if defaults == nil {
		defaults = func() SecurityPolicy { return SecurityPolicy{} }
	}
	return &CachedSecurityPolicyReader{
		reader:   NewCachedSettingReader(store, SecurityPolicySettingKey, ttl, defaults, ApplySecurityPolicySetting),
		defaults: defaults,
	}
}

func (r *CachedSecurityPolicyReader) SecurityPolicy(ctx context.Context) SecurityPolicy {
	if r == nil || r.reader == nil {
		if r != nil && r.defaults != nil {
			return r.defaults()
		}
		return SecurityPolicy{}
	}
	return r.reader.Value(ctx)
}

func (r *CachedSecurityPolicyReader) Refresh(ctx context.Context) error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Refresh(ctx)
}

// DefaultSecurityPolicyHolder encapsulates per-service singleton wiring for
// DefaultSecurityPolicyReader and SetSecurityPolicyStore without repeating
// mutex/once boilerplate in every service.
type DefaultSecurityPolicyHolder struct {
	defaults      func() SecurityPolicy
	fallbackStore func() SecurityPolicyStore

	mu     sync.RWMutex
	reader *CachedSecurityPolicyReader
	store  SecurityPolicyStore
}

func NewDefaultSecurityPolicyHolder(
	defaults func() SecurityPolicy,
	fallbackStore func() SecurityPolicyStore,
) *DefaultSecurityPolicyHolder {
	return &DefaultSecurityPolicyHolder{
		defaults:      defaults,
		fallbackStore: fallbackStore,
	}
}

func (h *DefaultSecurityPolicyHolder) Store() SecurityPolicyStore {
	return holderSecurityPolicyStore{holder: h}
}

func (h *DefaultSecurityPolicyHolder) Reader() *CachedSecurityPolicyReader {
	h.mu.RLock()
	if h.reader != nil {
		reader := h.reader
		h.mu.RUnlock()
		return reader
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reader == nil {
		h.reader = NewCachedSecurityPolicyReader(h.Store(), 30*time.Second, h.defaults)
	}
	return h.reader
}

func (h *DefaultSecurityPolicyHolder) SetReader(reader *CachedSecurityPolicyReader) func() {
	h.mu.Lock()
	previous := h.reader
	h.reader = reader
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		h.reader = previous
		h.mu.Unlock()
	}
}

func (h *DefaultSecurityPolicyHolder) SetStore(store SecurityPolicyStore) func() {
	h.mu.Lock()
	previous := h.store
	h.store = store
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		h.store = previous
		h.mu.Unlock()
	}
}

type holderSecurityPolicyStore struct {
	holder *DefaultSecurityPolicyHolder
}

func (s holderSecurityPolicyStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	s.holder.mu.RLock()
	store := s.holder.store
	s.holder.mu.RUnlock()
	if store != nil {
		return store.GetByKeyContext(ctx, key)
	}
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	if s.holder.fallbackStore == nil {
		return nil, ErrStoreUnavailable
	}
	return s.holder.fallbackStore().GetByKeyContext(ctx, key)
}

func ApplySecurityPolicySetting(policy SecurityPolicy, value map[string]any) SecurityPolicy {
	if value == nil {
		return policy
	}
	policy.PasswordMaxAgeDays = NonNegativeSetting(value, "password_max_age_days", policy.PasswordMaxAgeDays)
	policy.PasswordHistoryCount = NonNegativeSetting(value, "password_history_count", policy.PasswordHistoryCount)
	policy.LoginLimitMaxFailures = PositiveSetting(value, "login_limit_max_failures", policy.LoginLimitMaxFailures)
	policy.LoginLimitWindowMinutes = PositiveSetting(value, "login_limit_window_minutes", policy.LoginLimitWindowMinutes)
	policy.LoginLimitLockMinutes = PositiveSetting(value, "login_limit_lock_minutes", policy.LoginLimitLockMinutes)
	policy.LoginIPShieldMaxFailures = PositiveSetting(value, "login_ip_shield_max_failures", policy.LoginIPShieldMaxFailures)
	policy.LoginIPShieldWindowMinutes = PositiveSetting(value, "login_ip_shield_window_minutes", policy.LoginIPShieldWindowMinutes)
	policy.LoginIPShieldBlockMinutes = PositiveSetting(value, "login_ip_shield_block_minutes", policy.LoginIPShieldBlockMinutes)
	if enabled, ok := BoolSetting(value["login_alert_enabled"]); ok {
		policy.LoginAlertEnabled = enabled
	}
	if rps, ok := PositiveInt(value["rate_limit_rps"]); ok {
		policy.RateLimitWindowSeconds = 1
		policy.RateLimitMaxRequests = rps
	}
	return policy
}

func BoolSetting(value any) (bool, bool) {
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
