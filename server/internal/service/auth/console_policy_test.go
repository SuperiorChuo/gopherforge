package auth

import (
	"testing"

	"github.com/go-admin-kit/server/internal/config"
)

func TestDefaultConsoleSecurityPolicyUsesConfiguredValues(t *testing.T) {
	withConsolePolicyConfig(t, config.Config{
		JWT: config.JWTConfig{AccessTokenExpire: 30},
		Security: config.SecurityConfig{
			LoginLimit: config.LoginLimitConfig{
				MaxFailures: 9,
				LockMinutes: 12,
			},
		},
	})

	policy := DefaultConsoleSecurityPolicy()
	if policy.SessionTTLMinutes != 30 {
		t.Fatalf("SessionTTLMinutes = %d, want 30", policy.SessionTTLMinutes)
	}
	if policy.LoginMaxAttemptsPerHour != 9 {
		t.Fatalf("LoginMaxAttemptsPerHour = %d, want 9", policy.LoginMaxAttemptsPerHour)
	}
	if policy.LockoutMinutes != 12 {
		t.Fatalf("LockoutMinutes = %d, want 12", policy.LockoutMinutes)
	}
}

func TestDefaultConsoleSecurityPolicyUsesFallbacks(t *testing.T) {
	withConsolePolicyConfig(t, config.Config{})

	policy := DefaultConsoleSecurityPolicy()
	if policy.SessionTTLMinutes != 480 {
		t.Fatalf("SessionTTLMinutes = %d, want 480", policy.SessionTTLMinutes)
	}
	if policy.LoginMaxAttemptsPerHour != 5 {
		t.Fatalf("LoginMaxAttemptsPerHour = %d, want 5", policy.LoginMaxAttemptsPerHour)
	}
	if policy.LockoutMinutes != 15 {
		t.Fatalf("LockoutMinutes = %d, want 15", policy.LockoutMinutes)
	}
}

func TestSecureConsoleCookie(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want bool
	}{
		{
			name: "production enables secure cookies",
			cfg:  config.Config{App: config.AppCfg{Env: "production"}},
			want: true,
		},
		{
			name: "hsts enables secure cookies",
			cfg: config.Config{
				App: config.AppCfg{Env: "development"},
				Security: config.SecurityConfig{
					Headers: config.SecurityHeaders{HSTS: true},
				},
			},
			want: true,
		},
		{
			name: "development without hsts leaves secure disabled",
			cfg:  config.Config{App: config.AppCfg{Env: "development"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withConsolePolicyConfig(t, tt.cfg)

			if got := SecureConsoleCookie(); got != tt.want {
				t.Fatalf("SecureConsoleCookie() = %v, want %v", got, tt.want)
			}
		})
	}
}

func withConsolePolicyConfig(t *testing.T, cfg config.Config) {
	t.Helper()

	oldConfig := config.Cfg
	config.Cfg = cfg
	t.Cleanup(func() {
		config.Cfg = oldConfig
	})
}
