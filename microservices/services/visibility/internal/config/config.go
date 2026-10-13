// Package config provides environment-only configuration for visibility-service.
package config

import (
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	AppPort    string
	AppEnv     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	JWTSecret  string
	// Optional OpenAI-compatible endpoint for GEO runs.
	AIBaseURL string
	AIAPIKey  string
	AIModel   string
	// Perplexity / Gemini 真实引擎（可选）：配置后 engine=perplexity/gemini 可用。
	// 如网络需要代理，可配置可达渠道的 base_url + key。
	PerplexityBaseURL string
	PerplexityAPIKey  string
	PerplexityModel   string
	GeminiBaseURL     string
	GeminiAPIKey      string
	GeminiModel       string
	// SERP 采集：每日定时 + 手动触发。
	SerpEnabled  bool
	SerpCron     string // 标准 5 段 cron，本地时区（容器 TZ）
	SerpMaxPages int    // 每引擎最多翻页数，1 页 10 条
	// SerpEngines 启用的采集平台（逗号分隔）：
	// bing / baidu / google / sogou / so360 / duckduckgo / github
	SerpEngines []string
	// GEO 提及检测：每周定时自动跑真实 llm 检测。
	GeoCronEnabled bool
	GeoCron        string
	// 检测完成后的站内信送达（notify-service 内部端点）。
	NotifyAPIBase       string
	NotifyInternalToken string
	// Phase 2C：identity owner API（收件人管理员列表，替代直查共享 users 表）。
	IdentityAPIBase       string
	IdentityInternalToken string
	// DemoMode 演示假数据总闸。开 = 允许 stub 模式检测（模板拼的假回答）、
	// 排行榜在无真实检测时回落 stub 数据、演示数据播种端点挂载。
	// 非生产默认开（本地体验 / visibility-verify 冒烟依赖），生产默认关——
	// stub 回答的模板必然带上自有品牌名，被当成真实检测结果会直接误导决策。
	// 公开演示站需要时用 VISIBILITY_DEMO_MODE=true 显式打开。
	DemoMode bool
}

func Load() Config {
	c := build()
	if err := validate(c); err != nil {
		log.Fatalf("config: %v", err)
	}
	for _, w := range sanitize(&c) {
		log.Printf("WARNING: %s", w)
	}
	return c
}

func build() Config {
	appEnv := getenv("APP_ENV", "development")
	return Config{
		AppPort:               getenv("APP_PORT", "8093"),
		AppEnv:                appEnv,
		DBHost:                getenv("DB_HOST", "127.0.0.1"),
		DBPort:                getenv("DB_PORT", "5432"),
		DBUser:                getenv("DB_USER", "postgres"),
		DBPassword:            getSecret("DB_PASSWORD", "123456"),
		DBName:                getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:             getenv("DB_SSLMODE", "disable"),
		JWTSecret:             getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		AIBaseURL:             getenv("AI_BASE_URL", getenv("OPENAI_BASE_URL", "https://api.openai.com")),
		AIAPIKey:              getSecret("AI_API_KEY", getSecret("OPENAI_API_KEY", "")),
		AIModel:               getenv("AI_MODEL", "gpt-4o-mini"),
		PerplexityBaseURL:     getenv("PERPLEXITY_BASE_URL", "https://api.perplexity.ai"),
		PerplexityAPIKey:      getSecret("PERPLEXITY_API_KEY", ""),
		PerplexityModel:       getenv("PERPLEXITY_MODEL", "sonar"),
		GeminiBaseURL:         getenv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		GeminiAPIKey:          getSecret("GEMINI_API_KEY", ""),
		GeminiModel:           getenv("GEMINI_MODEL", "gemini-2.5-flash"),
		SerpEnabled:           getenv("SERP_ENABLED", "true") != "false",
		SerpCron:              getenv("SERP_CRON", "17 3 * * *"),
		SerpMaxPages:          getenvInt("SERP_MAX_PAGES", 5),
		SerpEngines:           splitCSV(getenv("SERP_ENGINES", "bing,baidu,google,sogou,so360,github")),
		GeoCronEnabled:        getenv("GEO_CRON_ENABLED", "true") != "false",
		GeoCron:               getenv("GEO_CRON", "43 4 * * 1"),
		NotifyAPIBase:         getenv("NOTIFY_API_BASE", ""),
		NotifyInternalToken:   getSecret("NOTIFY_INTERNAL_TOKEN", ""),
		IdentityAPIBase:       getenv("IDENTITY_API_BASE", "http://go-admin-kit-identity:8083"),
		IdentityInternalToken: getSecret("INTERNAL_TOKEN", ""),
		DemoMode:              demoModeEnabled("VISIBILITY_DEMO_MODE", appEnv),
	}
}

// validate 只硬拦"没有降级语义"的配置：JWT_SECRET 与 DB_PASSWORD。密钥不对
// 就没有能安全退化的行为，只能拒绝启动。口径与 auth、monitor 等核心服务一致
// （占位符黑名单 + 弱值黑名单 + 32 位门槛）。轻量服务不引 shared 依赖
// （Docker 构建上下文只含本服务目录），故为本地副本。
//
// 其余鉴权密钥一律不阻断启动，改为在使用点 fail closed 或降级告警（见
// sanitize）：阻断启动会把"少配一个可选 token"放大成整个服务起不来。
func validate(c Config) error {
	if !isProductionEnv(c.AppEnv) {
		return nil
	}
	issues := make([]string, 0)

	if !isStrongSecret(c.JWTSecret, 32) {
		issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
	}
	if isWeakCredential(c.DBPassword) {
		issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
	}

	if len(issues) > 0 {
		return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
	}
	return nil
}

// sanitize 生产环境下处理"配了等于没配"的密钥（空/占位符/弱值），返回需要打
// WARNING 的说明；一律不阻断启动。
//
// 两类区别对待：
//   - 入站鉴权闸门（别人拿凭据来调我）：显式归零，让使用点既有的"空=拒绝"
//     分支 fail closed，绝不拿开发占位符当真凭据校验。
//   - 出站凭据（我拿凭据去调别人）：只告警不归零。抹掉只会悄悄断掉功能，
//     安全与否取决于接收方是否校验，不该由调用方单方面降级。
func sanitize(c *Config) []string {
	if !isProductionEnv(c.AppEnv) {
		return nil
	}
	warnings := make([]string, 0)
	describe := func(name, value, consequence string) string {
		if strings.TrimSpace(value) != "" {
			return name + " is a placeholder or weak value; " + consequence
		}
		return name + " is not set; " + consequence
	}
	notice := func(name, value, consequence string) {
		if isWeakCredential(value) {
			warnings = append(warnings, describe(name, value, consequence))
		}
	}
	notice("NOTIFY_INTERNAL_TOKEN", c.NotifyInternalToken, "inbox delivery may be rejected by notify-service")
	return warnings
}

// ProductionMode 供使用点判断是否要 fail closed。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) }

// demoModeEnabled 演示假数据开关：显式取值优先（演示站可在生产 env 下打开），
// 未设置时按环境推导——非生产开、生产关。宁可少一个演示按钮，也不能让生产
// 用户把模板拼的假数据当成真实结果。
func demoModeEnabled(key, appEnv string) bool {
	switch secretstrength.NormalizeSecretValue(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return !isProductionEnv(appEnv)
} // 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func (c Config) DSN() string {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
	// Phase 2B：schema-per-service——search_path 读 DB_SEARCH_PATH env（空则不追加）
	if sp := os.Getenv("DB_SEARCH_PATH"); sp != "" {
		dsn += " search_path=" + sp
	}
	return dsn
}

func getSecret(key, fallback string) string {
	return envsecret.Get(key, fallback)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
