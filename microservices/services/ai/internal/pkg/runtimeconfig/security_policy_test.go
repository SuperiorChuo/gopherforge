package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/ai/internal/config"
	"github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
)

func TestSecurityPolicyReaderFallsBackToStaticConfig(t *testing.T) {
	oldSecurity := config.Cfg.Security
	config.Cfg.Security.PasswordMaxAgeDays = 90
	config.Cfg.Security.PasswordHistoryCount = 5
	config.Cfg.Security.LoginLimit.Enabled = true
	config.Cfg.Security.LoginLimit.MaxFailures = 6
	config.Cfg.Security.LoginLimit.WindowMinutes = 20
	config.Cfg.Security.LoginLimit.LockMinutes = 40
	config.Cfg.Security.RateLimit.Enabled = true
	config.Cfg.Security.RateLimit.WindowSeconds = 2
	config.Cfg.Security.RateLimit.MaxRequests = 200
	t.Cleanup(func() {
		config.Cfg.Security = oldSecurity
	})

	reader := NewCachedSecurityPolicyReader(&stubSecurityPolicyStore{err: gorm.ErrRecordNotFound}, time.Minute)
	policy := reader.SecurityPolicy(context.Background())

	if policy.PasswordMaxAgeDays != 90 || policy.PasswordHistoryCount != 5 {
		t.Fatalf("password policy = %#v, want static config values", policy)
	}
	if !policy.LoginLimitEnabled || policy.LoginLimitMaxFailures != 6 || policy.LoginLimitWindowMinutes != 20 || policy.LoginLimitLockMinutes != 40 {
		t.Fatalf("login limit policy = %#v, want static config values", policy)
	}
	if !policy.RateLimitEnabled || policy.RateLimitWindowSeconds != 2 || policy.RateLimitMaxRequests != 200 {
		t.Fatalf("rate limit policy = %#v, want static config values", policy)
	}
}

func TestSecurityPolicyReaderAppliesSecurityPolicySetting(t *testing.T) {
	oldSecurity := config.Cfg.Security
	config.Cfg.Security.PasswordMaxAgeDays = 90
	config.Cfg.Security.PasswordHistoryCount = 5
	config.Cfg.Security.LoginLimit.Enabled = true
	config.Cfg.Security.LoginLimit.MaxFailures = 6
	config.Cfg.Security.LoginLimit.WindowMinutes = 20
	config.Cfg.Security.LoginLimit.LockMinutes = 40
	config.Cfg.Security.RateLimit.Enabled = true
	config.Cfg.Security.RateLimit.WindowSeconds = 2
	config.Cfg.Security.RateLimit.MaxRequests = 200
	t.Cleanup(func() {
		config.Cfg.Security = oldSecurity
	})

	store := &stubSecurityPolicyStore{setting: &model.SystemSetting{
		SettingKey: SecurityPolicySettingKey,
		ValueJSON: map[string]any{
			"password_max_age_days":      float64(30),
			"password_history_count":     float64(8),
			"login_limit_max_failures":   float64(3),
			"login_limit_window_minutes": float64(10),
			"login_limit_lock_minutes":   float64(15),
			"rate_limit_rps":             float64(25),
		},
	}}
	reader := NewCachedSecurityPolicyReader(store, time.Minute)
	policy := reader.SecurityPolicy(context.Background())

	if policy.PasswordMaxAgeDays != 30 || policy.PasswordHistoryCount != 8 {
		t.Fatalf("password policy = %#v, want setting overrides", policy)
	}
	if policy.LoginLimitMaxFailures != 3 || policy.LoginLimitWindowMinutes != 10 || policy.LoginLimitLockMinutes != 15 {
		t.Fatalf("login limit policy = %#v, want setting overrides", policy)
	}
	if policy.RateLimitWindowSeconds != 1 || policy.RateLimitMaxRequests != 25 {
		t.Fatalf("rate limit policy = %#v, want rate_limit_rps mapped to 1s window", policy)
	}
}

func TestSecurityPolicyReaderKeepsFallbackForInvalidValues(t *testing.T) {
	oldSecurity := config.Cfg.Security
	config.Cfg.Security.PasswordMaxAgeDays = 90
	config.Cfg.Security.PasswordHistoryCount = 5
	config.Cfg.Security.LoginLimit.MaxFailures = 6
	config.Cfg.Security.RateLimit.WindowSeconds = 2
	config.Cfg.Security.RateLimit.MaxRequests = 200
	t.Cleanup(func() {
		config.Cfg.Security = oldSecurity
	})

	store := &stubSecurityPolicyStore{setting: &model.SystemSetting{
		SettingKey: SecurityPolicySettingKey,
		ValueJSON: map[string]any{
			"password_max_age_days":    float64(-1),
			"password_history_count":   "many",
			"login_limit_max_failures": float64(0),
			"rate_limit_rps":           float64(-10),
		},
	}}
	reader := NewCachedSecurityPolicyReader(store, time.Minute)
	policy := reader.SecurityPolicy(context.Background())

	if policy.PasswordMaxAgeDays != 90 || policy.PasswordHistoryCount != 5 || policy.LoginLimitMaxFailures != 6 || policy.RateLimitMaxRequests != 200 {
		t.Fatalf("policy = %#v, want invalid setting values ignored", policy)
	}
}

func TestSecurityPolicyReaderCachesAndRefreshes(t *testing.T) {
	store := &stubSecurityPolicyStore{setting: &model.SystemSetting{
		SettingKey: SecurityPolicySettingKey,
		ValueJSON:  map[string]any{"password_history_count": float64(3)},
	}}
	reader := NewCachedSecurityPolicyReader(store, time.Hour)

	if got := reader.SecurityPolicy(context.Background()).PasswordHistoryCount; got != 3 {
		t.Fatalf("initial password history count = %d, want 3", got)
	}
	store.setting.ValueJSON = map[string]any{"password_history_count": float64(9)}
	if got := reader.SecurityPolicy(context.Background()).PasswordHistoryCount; got != 3 {
		t.Fatalf("cached password history count = %d, want 3", got)
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want cache hit after first read", store.calls)
	}

	if err := reader.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got := reader.SecurityPolicy(context.Background()).PasswordHistoryCount; got != 9 {
		t.Fatalf("refreshed password history count = %d, want 9", got)
	}
}

type stubSecurityPolicyStore struct {
	setting *model.SystemSetting
	err     error
	calls   int
}

func (s *stubSecurityPolicyStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.setting == nil {
		return nil, errors.New("missing setting")
	}
	return s.setting, nil
}
