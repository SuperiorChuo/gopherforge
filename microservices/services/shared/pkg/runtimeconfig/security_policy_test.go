package runtimeconfig

import (
	"context"
	"testing"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

type stubSecurityPolicyStore struct {
	setting *model.SystemSetting
	err     error
}

func (s stubSecurityPolicyStore) GetByKeyContext(_ context.Context, _ string) (*model.SystemSetting, error) {
	return s.setting, s.err
}

func TestApplySecurityPolicySetting(t *testing.T) {
	base := SecurityPolicy{
		PasswordMaxAgeDays:         90,
		PasswordHistoryCount:       5,
		LoginLimitEnabled:          true,
		LoginLimitMaxFailures:      5,
		LoginLimitWindowMinutes:    15,
		LoginLimitLockMinutes:      30,
		LoginIPShieldMaxFailures:   30,
		LoginIPShieldWindowMinutes: 10,
		LoginIPShieldBlockMinutes:  10,
		LoginAlertEnabled:          true,
		RateLimitEnabled:           true,
		RateLimitWindowSeconds:     1,
		RateLimitMaxRequests:       100,
	}

	got := ApplySecurityPolicySetting(base, map[string]any{
		"password_max_age_days":          0,
		"password_history_count":         3,
		"login_limit_max_failures":       8,
		"login_limit_window_minutes":     20,
		"login_limit_lock_minutes":       45,
		"login_ip_shield_max_failures":   50,
		"login_ip_shield_window_minutes": 15,
		"login_ip_shield_block_minutes":  25,
		"login_alert_enabled":            "false",
		"rate_limit_rps":                 250,
	})

	if got.PasswordMaxAgeDays != 0 || got.PasswordHistoryCount != 3 {
		t.Fatalf("password settings = %+v", got)
	}
	if got.LoginLimitMaxFailures != 8 || got.LoginLimitWindowMinutes != 20 || got.LoginLimitLockMinutes != 45 {
		t.Fatalf("login limit settings = %+v", got)
	}
	if got.LoginIPShieldMaxFailures != 50 || got.LoginIPShieldWindowMinutes != 15 || got.LoginIPShieldBlockMinutes != 25 {
		t.Fatalf("ip shield settings = %+v", got)
	}
	if got.LoginAlertEnabled {
		t.Fatalf("expected LoginAlertEnabled=false")
	}
	if got.RateLimitWindowSeconds != 1 || got.RateLimitMaxRequests != 250 {
		t.Fatalf("rate limit settings = %+v", got)
	}
}

func TestDefaultSecurityPolicyHolder(t *testing.T) {
	holder := NewDefaultSecurityPolicyHolder(
		func() SecurityPolicy {
			return SecurityPolicy{LoginLimitMaxFailures: 5, LoginAlertEnabled: true}
		},
		nil,
	)
	restore := holder.SetStore(stubSecurityPolicyStore{
		setting: &model.SystemSetting{
			SettingKey: SecurityPolicySettingKey,
			ValueJSON: map[string]any{
				"login_limit_max_failures": float64(9),
				"login_alert_enabled":      false,
			},
		},
	})
	defer restore()

	policy := holder.Reader().SecurityPolicy(context.Background())
	if policy.LoginLimitMaxFailures != 9 || policy.LoginAlertEnabled {
		t.Fatalf("policy from holder = %+v", policy)
	}

	reader := NewCachedSecurityPolicyReader(stubSecurityPolicyStore{err: ErrStoreUnavailable}, time.Minute, func() SecurityPolicy {
		return SecurityPolicy{LoginLimitMaxFailures: 7}
	})
	if fallback := reader.SecurityPolicy(context.Background()); fallback.LoginLimitMaxFailures != 7 {
		t.Fatalf("fallback policy = %+v", fallback)
	}
}
