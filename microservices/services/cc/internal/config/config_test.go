package config

import (
	"strings"
	"testing"
)

// prodConfig 生产环境下的"全部配齐"基线，各用例只改一项来断言该项的行为。
func prodConfig() Config {
	return Config{
		AppEnv:              "production",
		JWTSecret:           "test-jwt-secret-for-unit-tests-0123456789",
		DBPassword:          "test-db-password-for-unit-tests",
		WebhookSecret:       "test-cc-webhook-token",
		CDRAuth:             "fsuser:9c2e7a41f0",
		FSAPIBase:           "http://control-api:8090",
		FSAPIToken:          "test-fs-api-token",
		CRMAPIBase:          "http://go-admin-kit-crm:8091",
		CRMInternalToken:    "test-crm-internal-token",
		NotifyAPIBase:       "http://go-admin-kit-notify:8095",
		NotifyInternalToken: "test-notify-internal-token",
	}
}

func TestValidateAllowsDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	if err := validate(build()); err != nil {
		t.Fatalf("validate() error = %v, want nil for development defaults", err)
	}
}

// JWT_SECRET / DB_PASSWORD 没有安全降级语义，占位符必须拒绝启动。
func TestValidateRejectsPlaceholderJWTSecret(t *testing.T) {
	c := prodConfig()
	c.JWTSecret = "local-dev-secret-change-me-32-chars"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("validate() error = %v, want JWT_SECRET placeholder error", err)
	}
}

func TestValidateRejectsShortJWTSecret(t *testing.T) {
	c := prodConfig()
	c.JWTSecret = "7a3f9c1e5b8d2046"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("validate() error = %v, want JWT_SECRET length error", err)
	}
}

func TestValidateRejectsDefaultDBPassword(t *testing.T) {
	c := prodConfig()
	c.DBPassword = "123456"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("validate() error = %v, want DB_PASSWORD weak error", err)
	}
}

// 关键回归：缺 webhook / CDR 密钥不阻断其他已配置功能启动，
// 改由端点 fail closed 兜底。
func TestValidateAllowsMissingWebhookAndCDRSecrets(t *testing.T) {
	c := prodConfig()
	c.WebhookSecret = ""
	c.CDRAuth = ""
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — missing gate secrets must not block startup", err)
	}
}

// 缺 / 占位的入站闸门密钥被归零并告警，使用点据此关端点。
func TestSanitizeBlanksGateSecretsAndWarns(t *testing.T) {
	c := prodConfig()
	c.WebhookSecret = ""
	c.CDRAuth = "change-me"

	warnings := sanitize(&c)

	if c.CDRAuth != "" {
		t.Fatalf("CDRAuth = %q, want blanked so the endpoint fails closed", c.CDRAuth)
	}
	joined := strings.Join(warnings, " | ")
	for _, want := range []string{"CC_WEBHOOK_SECRET", "CC_CDR_AUTH"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings = %q, want it to mention %s", joined, want)
		}
	}
}

func TestSanitizeKeepsRealSecrets(t *testing.T) {
	c := prodConfig()
	if warnings := sanitize(&c); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a fully configured production config", warnings)
	}
	if c.WebhookSecret == "" || c.CDRAuth == "" {
		t.Fatal("sanitize must not blank real secrets")
	}
}

// 开发环境保持原样：空密钥仍是"免鉴权"的本地便利，不归零、不告警。
func TestSanitizeIsNoopOutsideProduction(t *testing.T) {
	c := prodConfig()
	c.AppEnv = "development"
	c.WebhookSecret = ""
	c.CDRAuth = ""
	if warnings := sanitize(&c); warnings != nil {
		t.Fatalf("warnings = %v, want nil outside production", warnings)
	}
}

// 对接基址为空 = 该对接整体关闭，不该告警。
func TestSanitizeSkipsTokensForDisabledIntegrations(t *testing.T) {
	c := prodConfig()
	c.FSAPIBase, c.FSAPIToken = "", ""
	c.CRMAPIBase, c.CRMInternalToken = "", ""
	c.NotifyAPIBase, c.NotifyInternalToken = "", ""
	if warnings := sanitize(&c); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none when integrations are disabled", warnings)
	}
}

func TestSanitizeWarnsForEnabledIntegrationsMissingTokens(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"fs", func(c *Config) { c.FSAPIToken = "" }, "CC_FS_TOKEN"},
		{"crm", func(c *Config) { c.CRMInternalToken = "change-me" }, "CC_CRM_INTERNAL_TOKEN"},
		{"notify", func(c *Config) { c.NotifyInternalToken = "" }, "CC_NOTIFY_INTERNAL_TOKEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := prodConfig()
			tc.mutate(&c)
			warnings := sanitize(&c)
			if !strings.Contains(strings.Join(warnings, " | "), tc.want) {
				t.Fatalf("warnings = %v, want %s warning", warnings, tc.want)
			}
			if err := validate(c); err != nil {
				t.Fatalf("validate() error = %v, want nil — token issues must not block startup", err)
			}
		})
	}
}

// compose 注入的开发占位符不能被当成真凭据。
func TestPlaceholderBlacklistCoversDeployedDevTokens(t *testing.T) {
	for _, v := range []string{"dev-notify-internal-token", "dev-im-ai-internal-token"} {
		if !isPlaceholderValue(v) {
			t.Fatalf("isPlaceholderValue(%q) = false, want true", v)
		}
	}
}

func TestIsProductionEnvIsCaseInsensitive(t *testing.T) {
	for _, env := range []string{"production", "Production", " PRODUCTION "} {
		if !isProductionEnv(env) {
			t.Fatalf("isProductionEnv(%q) = false, want true", env)
		}
	}
	for _, env := range []string{"development", "staging", ""} {
		if isProductionEnv(env) {
			t.Fatalf("isProductionEnv(%q) = true, want false", env)
		}
	}
}
