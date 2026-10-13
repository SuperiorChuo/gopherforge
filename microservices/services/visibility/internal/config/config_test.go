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
		AppEnv:              "production",
		JWTSecret:           "test-jwt-secret-for-unit-tests-0123456789",
		DBPassword:          "test-db-password-for-unit-tests",
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
	c.NotifyInternalToken = testDevToken
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — placeholder tokens must not block startup", err)
	}
}

// 缺 token 同样不阻断启动。
func TestValidateAllowsEmptyTokens(t *testing.T) {
	c := prodConfig()
	c.NotifyInternalToken = ""
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — missing tokens must not block startup", err)
	}
}

func TestSanitizeWarnsForPlaceholderTokens(t *testing.T) {
	c := prodConfig()
	c.NotifyInternalToken = testDevToken
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
	c.NotifyInternalToken = testDevToken
	if warnings := sanitize(&c); warnings != nil {
		t.Fatalf("warnings = %v, want nil outside production", warnings)
	}
}

// 出站凭据只告警不归零：抹掉只会悄悄断掉功能，安全与否取决于接收方。
func TestSanitizeKeepsOutboundCredentials(t *testing.T) {
	c := prodConfig()
	c.NotifyInternalToken = testDevToken

	sanitize(&c)

	if c.NotifyInternalToken == "" {
		t.Fatal("NotifyInternalToken must not be blanked — it is an outbound credential")
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

// ---------- 演示假数据闸门（VISIBILITY_DEMO_MODE）----------
//
// stub 检测的回答由模板拼出、必然带上自有品牌名，被当成真实检测结果会直接
// 误导品牌决策。生产默认关：stub 模式被拒、排行榜不回落演示数据、
// seed-demo 路由不挂载。

func TestDemoModeDefaultsOffInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("VISIBILITY_DEMO_MODE", "")
	if build().DemoMode {
		t.Fatal("DemoMode = true, want false by default under APP_ENV=production")
	}
}

func TestDemoModeDefaultsOnOutsideProduction(t *testing.T) {
	t.Setenv("VISIBILITY_DEMO_MODE", "")
	for _, env := range []string{"development", "test", "staging"} {
		t.Setenv("APP_ENV", env)
		if !build().DemoMode {
			t.Fatalf("DemoMode = false for APP_ENV=%s, want true", env)
		}
	}
}

// 演示站要能在生产 env 下显式打开。
func TestDemoModeExplicitOverrideWins(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	for _, v := range []string{"true", "1", "yes", "on", "TRUE"} {
		t.Setenv("VISIBILITY_DEMO_MODE", v)
		if !build().DemoMode {
			t.Fatalf("DemoMode = false for VISIBILITY_DEMO_MODE=%q, want true", v)
		}
	}
	t.Setenv("APP_ENV", "development")
	for _, v := range []string{"false", "0", "no", "off"} {
		t.Setenv("VISIBILITY_DEMO_MODE", v)
		if build().DemoMode {
			t.Fatalf("DemoMode = true for VISIBILITY_DEMO_MODE=%q, want false", v)
		}
	}
}

func TestDemoModeGarbageValueFallsBackToEnvDefault(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("VISIBILITY_DEMO_MODE", "maybe")
	if build().DemoMode {
		t.Fatal("DemoMode = true for VISIBILITY_DEMO_MODE=maybe under production, want false")
	}
}
