package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDatabaseConfigConnectionPoolLifetimeDefaults(t *testing.T) {
	cfg := DatabaseConfig{}

	if got := cfg.EffectiveConnMaxLifetime(); got != 5*time.Minute {
		t.Fatalf("conn max lifetime = %s, want 5m", got)
	}
	if got := cfg.EffectiveConnMaxIdleTime(); got != 3*time.Minute {
		t.Fatalf("conn max idle time = %s, want 3m", got)
	}
}

func TestDatabaseConfigConnectionPoolLifetimeOverrides(t *testing.T) {
	cfg := DatabaseConfig{
		ConnMaxLifetimeSeconds: 120,
		ConnMaxIdleTimeSeconds: 45,
	}

	if got := cfg.EffectiveConnMaxLifetime(); got != 2*time.Minute {
		t.Fatalf("conn max lifetime = %s, want 2m", got)
	}
	if got := cfg.EffectiveConnMaxIdleTime(); got != 45*time.Second {
		t.Fatalf("conn max idle time = %s, want 45s", got)
	}
}

func TestSecurityConfigPasswordPolicyDefaults(t *testing.T) {
	cfg := SecurityConfig{}

	if got := cfg.EffectivePasswordMaxAgeDays(); got != 0 {
		t.Fatalf("password max age days = %d, want disabled", got)
	}
	if got := cfg.EffectivePasswordHistoryCount(); got != 0 {
		t.Fatalf("password history count = %d, want disabled", got)
	}
}

func TestSecurityConfigPasswordPolicyOverrides(t *testing.T) {
	cfg := SecurityConfig{
		PasswordMaxAgeDays:   90,
		PasswordHistoryCount: 8,
	}

	if got := cfg.EffectivePasswordMaxAgeDays(); got != 90 {
		t.Fatalf("password max age days = %d, want 90", got)
	}
	if got := cfg.EffectivePasswordHistoryCount(); got != 8 {
		t.Fatalf("password history count = %d, want 8", got)
	}
}

func TestOAuthProviderConfigDefaultsDisabled(t *testing.T) {
	cfg := OAuthProviderConfig{}

	if cfg.Ready() {
		t.Fatal("OAuth provider must default to disabled")
	}
}

func TestOAuthProviderConfigRequiresEnabledAndCredentials(t *testing.T) {
	cfg := OAuthProviderConfig{
		Enabled:      true,
		ClientID:     "github-client-id",
		ClientSecret: "github-client-secret",
		RedirectURI:  "http://localhost:8081/api/v1/oauth/github/callback",
	}
	if !cfg.Ready() {
		t.Fatal("OAuth provider with enabled real-looking credentials should be ready")
	}

	cfg.ClientSecret = "your-github-client-secret"
	if cfg.Ready() {
		t.Fatal("OAuth provider with placeholder secret must not be ready")
	}
}

func TestEmailConfigSupportsTLSYAMLFields(t *testing.T) {
	var cfg Config
	raw := []byte(`notification:
  email:
    enabled: false
    smtp_host: smtp.example.com
    smtp_port: 465
    use_tls: true
    start_tls: false
`)

	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if !cfg.Notification.Email.UseTLS {
		t.Fatal("use_tls YAML field should set EmailConfig.UseTLS")
	}
	if cfg.Notification.Email.StartTLS {
		t.Fatal("start_tls YAML field should set EmailConfig.StartTLS to false")
	}
}

func TestEmailNotificationConfigSupportsTemplateAndRecipientGroupsYAMLFields(t *testing.T) {
	var cfg Config
	raw := []byte(`notification:
  email:
    subject_template: "Notice {{id}}: {{title}}"
    body_template: "{{type}} {{content}}"
    recipient_groups:
      notice: [ops@example.com]
      audit:
        - audit@example.com
`)

	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if cfg.Notification.Email.SubjectTemplate != "Notice {{id}}: {{title}}" {
		t.Fatalf("subject_template = %q, want YAML value", cfg.Notification.Email.SubjectTemplate)
	}
	if cfg.Notification.Email.BodyTemplate != "{{type}} {{content}}" {
		t.Fatalf("body_template = %q, want YAML value", cfg.Notification.Email.BodyTemplate)
	}
	if got := cfg.Notification.Email.RecipientGroups["notice"]; len(got) != 1 || got[0] != "ops@example.com" {
		t.Fatalf("recipient_groups.notice = %#v, want YAML notice recipients", got)
	}
	if got := cfg.Notification.Email.RecipientGroups["audit"]; len(got) != 1 || got[0] != "audit@example.com" {
		t.Fatalf("recipient_groups.audit = %#v, want YAML audit recipients", got)
	}
}

func TestEmailNotificationReplaceEnvVarsAppliesTemplates(t *testing.T) {
	t.Setenv("EMAIL_SUBJECT_TEMPLATE", "Runtime {{title}}")
	t.Setenv("EMAIL_BODY_TEMPLATE", "Runtime body {{content}}")
	cfg := Config{
		Notification: NotificationConfig{
			Email: EmailConfig{
				SubjectTemplate: "Static subject",
				BodyTemplate:    "Static body",
			},
		},
	}

	replaceEnvVars(&cfg)

	if cfg.Notification.Email.SubjectTemplate != "Runtime {{title}}" {
		t.Fatalf("SubjectTemplate = %q, want env override", cfg.Notification.Email.SubjectTemplate)
	}
	if cfg.Notification.Email.BodyTemplate != "Runtime body {{content}}" {
		t.Fatalf("BodyTemplate = %q, want env override", cfg.Notification.Email.BodyTemplate)
	}
}

func TestReplaceEnvVarsAppliesEmailTLSModes(t *testing.T) {
	t.Setenv("EMAIL_USE_TLS", "false")
	t.Setenv("EMAIL_START_TLS", "true")
	cfg := Config{
		Notification: NotificationConfig{
			Email: EmailConfig{
				UseTLS:   true,
				StartTLS: false,
			},
		},
	}

	replaceEnvVars(&cfg)

	if cfg.Notification.Email.UseTLS {
		t.Fatal("EMAIL_USE_TLS=false should override UseTLS to false")
	}
	if !cfg.Notification.Email.StartTLS {
		t.Fatal("EMAIL_START_TLS=true should override StartTLS to true")
	}
}

func TestValidateRejectsConflictingEmailTLSModes(t *testing.T) {
	oldCfg := Cfg
	Cfg = Config{
		Notification: NotificationConfig{
			Email: EmailConfig{
				UseTLS:   true,
				StartTLS: true,
			},
		},
	}
	t.Cleanup(func() {
		Cfg = oldCfg
	})

	err := Validate()
	if err == nil || !strings.Contains(err.Error(), "notification.email.use_tls and notification.email.start_tls") {
		t.Fatalf("Validate() error = %v, want conflicting email TLS modes error", err)
	}
}

func TestValidateRejectsConflictingEmailTLSModesFromEnv(t *testing.T) {
	oldCfg := Cfg
	t.Cleanup(func() {
		Cfg = oldCfg
	})
	t.Setenv("EMAIL_START_TLS", "true")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`notification:
  email:
    use_tls: true
    start_tls: false
`)
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	err := Validate()
	if err == nil || !strings.Contains(err.Error(), "notification.email.use_tls and notification.email.start_tls") {
		t.Fatalf("Validate() error = %v, want conflicting email TLS modes error", err)
	}
}
