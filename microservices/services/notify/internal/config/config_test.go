package config

import (
	"strings"
	"testing"
)

// testDevToken is a published development placeholder, never a credential.
const testDevToken = "dev-notify-internal-token"

// prodConfig 生产环境下的合规基线，各用例只改一项来断言该项的行为。
func prodConfig() Config {
	return Config{
		AppEnv:        "production",
		JWTSecret:     "test-jwt-secret-for-unit-tests-0123456789",
		DBPassword:    "test-db-password-for-unit-tests",
		InternalToken: "test-notify-internal-token",
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
	c.JWTSecret = "8c3f7a1e5b2d9046"
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

func TestValidateAcceptsConfiguredProduction(t *testing.T) {
	if err := validate(prodConfig()); err != nil {
		t.Fatalf("validate() error = %v, want nil for a configured production config", err)
	}
}

// 关键回归：示例 Compose 可能注入 dev-notify-internal-token 这类占位符，
// 它们绝不能阻断启动——只降级 + 告警。
func TestValidateAllowsPlaceholderTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevToken
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — placeholder tokens must not block startup", err)
	}
}

// 缺 token 同样不阻断启动。
func TestValidateAllowsEmptyTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = ""
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — missing tokens must not block startup", err)
	}
}

func TestSanitizeWarnsForPlaceholderTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevToken
	warnings := sanitize(&c)
	joined := strings.Join(warnings, " | ")
	for _, want := range []string{"NOTIFY_INTERNAL_TOKEN"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings = %q, want it to mention %s", joined, want)
		}
	}
}

func TestSanitizeIsQuietWhenFullyConfigured(t *testing.T) {
	c := prodConfig()
	if warnings := sanitize(&c); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a configured production config", warnings)
	}
}

// 开发环境不做任何降级/告警。
func TestSanitizeIsNoopOutsideProduction(t *testing.T) {
	c := prodConfig()
	c.AppEnv = "development"
	c.InternalToken = testDevToken
	if warnings := sanitize(&c); warnings != nil {
		t.Fatalf("warnings = %v, want nil outside production", warnings)
	}
}

// 入站闸门：占位符被归零，使用点既有的"空=503"分支据此 fail closed，
// 绝不拿众所周知的开发 token 当真凭据校验。
func TestSanitizeBlanksInboundGateToken(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevToken

	sanitize(&c)

	if c.InternalToken != "" {
		t.Fatalf("InternalToken = %q, want blanked so internal endpoints fail closed", c.InternalToken)
	}
}

// compose 注入的开发占位符必须在黑名单里。
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

// ---------- 演示假通道闸门（NOTIFY_DEMO_MODE）----------
//
// 短信 stub 只写一行日志、从不真发（真实 provider 尚未实现）。非演示环境把它
// 降级成"未配置"，让 dispatcher 走 skipped 分支——告警链路上一条标着 stub 的
// 记录，比一条明确的 skipped 更容易让人以为短信已经发出去了。

func TestDemoModeDefaultsOffInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("NOTIFY_DEMO_MODE", "")
	if build().DemoMode {
		t.Fatal("DemoMode = true, want false by default under APP_ENV=production")
	}
}

func TestDemoModeDefaultsOnOutsideProduction(t *testing.T) {
	t.Setenv("NOTIFY_DEMO_MODE", "")
	for _, env := range []string{"development", "test", "staging"} {
		t.Setenv("APP_ENV", env)
		if !build().DemoMode {
			t.Fatalf("DemoMode = false for APP_ENV=%s, want true", env)
		}
	}
}

func TestDemoModeExplicitOverrideWins(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	for _, v := range []string{"true", "1", "yes", "on", "TRUE"} {
		t.Setenv("NOTIFY_DEMO_MODE", v)
		if !build().DemoMode {
			t.Fatalf("DemoMode = false for NOTIFY_DEMO_MODE=%q, want true", v)
		}
	}
	t.Setenv("APP_ENV", "development")
	for _, v := range []string{"false", "0", "no", "off"} {
		t.Setenv("NOTIFY_DEMO_MODE", v)
		if build().DemoMode {
			t.Fatalf("DemoMode = true for NOTIFY_DEMO_MODE=%q, want false", v)
		}
	}
}

// 生产环境即便有人配了 NOTIFY_SMS_MODE=stub，也不能启用只写日志的假通道。
func TestEffectiveSMSModeDropsStubOutsideDemoMode(t *testing.T) {
	c := Config{SMSMode: "stub", DemoMode: false}
	if got := c.EffectiveSMSMode(); got != "" {
		t.Fatalf("EffectiveSMSMode() = %q, want \"\" so the dispatcher reports skipped", got)
	}
}

func TestEffectiveSMSModeKeepsStubInDemoMode(t *testing.T) {
	c := Config{SMSMode: "stub", DemoMode: true}
	if got := c.EffectiveSMSMode(); got != "stub" {
		t.Fatalf("EffectiveSMSMode() = %q, want \"stub\" for demo deployments", got)
	}
}

// 没配就是没配，不该因为演示模式凭空冒出一个通道。
func TestEffectiveSMSModeStaysEmptyWhenUnset(t *testing.T) {
	if got := (Config{SMSMode: "", DemoMode: true}).EffectiveSMSMode(); got != "" {
		t.Fatalf("EffectiveSMSMode() = %q, want \"\"", got)
	}
}
