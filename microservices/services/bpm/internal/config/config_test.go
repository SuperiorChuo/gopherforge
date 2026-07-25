package config

import (
	"strings"
	"testing"
)

// testDevPlaceholder 是 compose 里公开的开发默认值之一，即"被校验拒绝的值"
// 本身，不是任何真实凭据。抽成常量而非到处写字面量：pre-commit 密钥扫描会把
// `Token = "长字符串"` 形态当疑似真实密钥拦下，走命名常量既避开误报，也让
// 用例读起来就是"占位符"而不是某个具体 token。
const testDevPlaceholder = "dev-notify-internal-token"

// prodConfig 生产环境下的合规基线，各用例只改一项来断言该项的行为。
// 夹具一律用 test- 前缀的显式测试值，不要写成拟真凭据（既会被密钥扫描
// 钩子误判，也会让读者以为是真密码）。
func prodConfig() Config {
	return Config{
		AppEnv:              "production",
		JWTSecret:           "test-jwt-secret-for-unit-tests-0123456789",
		DBPassword:          "test-db-password-for-unit-tests",
		InternalToken:       "test-bpm-internal-token",
		CallbackToken:       "test-bpm-callback-token",
		NotifyAPIBase:       "http://notify.test.invalid",
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

// 关键回归：compose 里的可选 token 可能仍是开发占位符或干脆没配，
// 它们绝不能阻断启动——只降级 + 告警。一律硬拦会让已有部署重启后起不来。
func TestValidateAllowsPlaceholderTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevPlaceholder
	c.CallbackToken = testDevPlaceholder
	c.NotifyInternalToken = testDevPlaceholder
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — placeholder tokens must not block startup", err)
	}
}

// 缺 token 同样不阻断启动。
func TestValidateAllowsEmptyTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = ""
	c.CallbackToken = ""
	c.NotifyAPIBase = ""
	c.NotifyInternalToken = ""
	if err := validate(c); err != nil {
		t.Fatalf("validate() error = %v, want nil — missing tokens must not block startup", err)
	}
}

func TestSanitizeWarnsForPlaceholderTokens(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevPlaceholder
	c.CallbackToken = testDevPlaceholder
	c.NotifyInternalToken = testDevPlaceholder
	warnings := sanitize(&c)
	joined := strings.Join(warnings, " | ")
	for _, want := range []string{"BPM_INTERNAL_TOKEN", "BPM_CALLBACK_TOKEN", "NOTIFY_INTERNAL_TOKEN"} {
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

// 站内信通道整体关闭（脚手架默认无通知服务）时不提 token 缺失，
// 否则每次生产启动都刷一条与实际无关的告警。
func TestSanitizeSkipsNotifyTokenWhenChannelDisabled(t *testing.T) {
	c := prodConfig()
	c.NotifyAPIBase = ""
	c.NotifyInternalToken = ""
	joined := strings.Join(sanitize(&c), " | ")
	if strings.Contains(joined, "NOTIFY_INTERNAL_TOKEN") {
		t.Fatalf("warnings = %q, want no NOTIFY_INTERNAL_TOKEN warning when NOTIFY_API_BASE is unset", joined)
	}
}

// 开发环境不做任何降级/告警。
func TestSanitizeIsNoopOutsideProduction(t *testing.T) {
	c := prodConfig()
	c.AppEnv = "development"
	c.InternalToken = testDevPlaceholder
	c.CallbackToken = testDevPlaceholder
	c.NotifyInternalToken = testDevPlaceholder
	if warnings := sanitize(&c); warnings != nil {
		t.Fatalf("warnings = %v, want nil outside production", warnings)
	}
}

// 入站闸门：占位符被归零，使用点既有的"空=503"分支据此 fail closed
// （见 internal/api.Server.requireInternal），绝不拿众所周知的开发 token
// 当真凭据校验。
func TestSanitizeBlanksInboundGateToken(t *testing.T) {
	c := prodConfig()
	c.InternalToken = testDevPlaceholder

	sanitize(&c)

	if c.InternalToken != "" {
		t.Fatalf("InternalToken = %q, want blanked so internal endpoints fail closed", c.InternalToken)
	}
}

// 出站凭据只告警不归零：抹掉只会悄悄断掉功能，安全与否取决于接收方。
func TestSanitizeKeepsOutboundCredentials(t *testing.T) {
	c := prodConfig()
	c.CallbackToken = testDevPlaceholder
	c.NotifyInternalToken = testDevPlaceholder

	sanitize(&c)

	if c.CallbackToken == "" {
		t.Fatal("CallbackToken must not be blanked — it is an outbound credential")
	}
	if c.NotifyInternalToken == "" {
		t.Fatal("NotifyInternalToken must not be blanked — it is an outbound credential")
	}
}

// compose / 上游发布过的开发占位符必须全部落进黑名单：
// dev-notify-internal-token 是本仓 compose 的 alertmanager 默认值；
// dev-im-ai-internal-token 来自上游完整产品，经 dev- 前缀兜底一并拒绝。
func TestPlaceholderBlacklistCoversPublishedDevTokens(t *testing.T) {
	for _, v := range []string{
		"dev-notify-internal-token",
		"dev-im-ai-internal-token",
		"local-dev-secret-change-me-32-chars",
	} {
		if !isPlaceholderValue(v) {
			t.Fatalf("isPlaceholderValue(%q) = false, want true", v)
		}
	}
}

// 弱值黑名单是精确匹配："test" 算弱，"test-…" 这类显式测试夹具不算，
// 否则本文件的 prodConfig 会自相矛盾。
func TestWeakCredentialMatchesExactValuesOnly(t *testing.T) {
	if !isWeakCredential("test") {
		t.Fatal(`isWeakCredential("test") = false, want true`)
	}
	if isWeakCredential("test-bpm-internal-token") {
		t.Fatal(`isWeakCredential("test-bpm-internal-token") = true, want false`)
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

func TestProductionModeReportsEnv(t *testing.T) {
	if !prodConfig().ProductionMode() {
		t.Fatal("ProductionMode() = false, want true for APP_ENV=production")
	}
	c := prodConfig()
	c.AppEnv = "development"
	if c.ProductionMode() {
		t.Fatal("ProductionMode() = true, want false for APP_ENV=development")
	}
}
