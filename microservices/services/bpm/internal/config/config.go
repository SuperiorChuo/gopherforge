// Package config 提供 bpm-service 的纯环境变量配置（轻量实验线服务约定，
// 与 ticket/crm/im 一致，不携带核心服务的完整配置框架）。
package config

import (
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
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
	// InternalToken 校验业务方（CRM 等）内网调用 bpm internal 端点的
	// X-Internal-Token；空=禁用 internal 端点（返回 503），不裸奔。
	InternalToken string
	// CallbackToken 终态回调业务方时携带的 X-Internal-Token（业务侧内部
	// 端点鉴权，如 CRM 的 CRM_INTERNAL_TOKEN）；空=回调不带鉴权头。
	CallbackToken string
	// Phase 2D：NATS 审计事件发布（audit.log.*）。
	NATSURL string
	// NotifyAPIBase + NotifyInternalToken：新待办/抄送/终态站内信；
	// token 空=静默跳过通知（不阻断审批主流程）。
	NotifyAPIBase       string
	NotifyInternalToken string
	// Phase 2C：identity owner API（按角色/部门/用户解析审批人，替代直查共享表）。
	IdentityAPIBase       string
	IdentityInternalToken string
	// TimeoutScanInterval 超时提醒扫描周期（BPM_TIMEOUT_SCAN_INTERVAL，
	// time.ParseDuration 语法，默认 5m）。
	TimeoutScanInterval time.Duration
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
	return Config{
		AppPort:               getenv("APP_PORT", "8096"),
		AppEnv:                getenv("APP_ENV", "development"),
		DBHost:                getenv("DB_HOST", "127.0.0.1"),
		DBPort:                getenv("DB_PORT", "5432"),
		DBUser:                getenv("DB_USER", "postgres"),
		DBPassword:            getSecret("DB_PASSWORD", "123456"),
		DBName:                getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:             getenv("DB_SSLMODE", "disable"),
		JWTSecret:             getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		InternalToken:         getSecret("BPM_INTERNAL_TOKEN", ""),
		CallbackToken:         getSecret("BPM_CALLBACK_TOKEN", ""),
		NATSURL:               getenv("NATS_URL", ""),
		NotifyAPIBase:         getenv("NOTIFY_API_BASE", "http://go-admin-kit-notify:8095"),
		NotifyInternalToken:   getSecret("NOTIFY_INTERNAL_TOKEN", ""),
		IdentityAPIBase:       getenv("IDENTITY_API_BASE", "http://go-admin-kit-identity:8083"),
		IdentityInternalToken: getSecret("INTERNAL_TOKEN", ""),
		TimeoutScanInterval:   getenvDuration("BPM_TIMEOUT_SCAN_INTERVAL", 5*time.Minute),
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
	gate := func(name string, field *string, consequence string) {
		if isWeakCredential(*field) {
			warnings = append(warnings, describe(name, *field, consequence))
			*field = ""
		}
	}
	notice := func(name, value, consequence string) {
		if isWeakCredential(value) {
			warnings = append(warnings, describe(name, value, consequence))
		}
	}
	gate("BPM_INTERNAL_TOKEN", &c.InternalToken, "internal endpoints reject all callers (503)")
	notice("BPM_CALLBACK_TOKEN", c.CallbackToken, "terminal-state callbacks are sent without an authentication header")
	if strings.TrimSpace(c.NotifyAPIBase) != "" {
		notice("NOTIFY_INTERNAL_TOKEN", c.NotifyInternalToken, "inbox push may be rejected by notify-service")
	}
	return warnings
}

// ProductionMode 供使用点判断是否要 fail closed。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) }

func isProductionEnv(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production")
}

func isStrongSecret(value string, minLength int) bool {
	value = strings.TrimSpace(value)
	return len(value) >= minLength && !isPlaceholderValue(value)
}

// isWeakCredential 空值、占位符、以及常见开发默认值都算弱。
func isWeakCredential(value string) bool {
	normalized := normalizeSecretValue(value)
	if normalized == "" || isPlaceholderValue(normalized) {
		return true
	}
	weakValues := map[string]struct{}{
		"123456":                {},
		"access-key":            {},
		"accesskey":             {},
		"admin":                 {},
		"aws-access-key-id":     {},
		"aws-secret-access-key": {},
		"aws_access_key_id":     {},
		"aws_secret_access_key": {},
		"awsaccesskeyid":        {},
		"awssecretaccesskey":    {},
		"changeme":              {},
		"default":               {},
		"demo":                  {},
		"development":           {},
		"example":               {},
		"go-admin-kit":          {},
		"local":                 {},
		"minioadmin":            {},
		"password":              {},
		"redis":                 {},
		"root":                  {},
		"sample":                {},
		"secret":                {},
		"secret-key":            {},
		"secretkey":             {},
		"test":                  {},
		"test123":               {},
	}
	if _, ok := weakValues[normalized]; ok {
		return true
	}
	// dev- 前缀按约定视为开发占位符（与 auth/monitor 等服务口径一致）——
	// 生产环境下 dev-xxx 形态的 token 不该被当成真凭据放行。
	return strings.HasPrefix(normalized, "dev-")
}

func isPlaceholderValue(value string) bool {
	normalized := normalizeSecretValue(value)
	if normalized == "" {
		return true
	}
	placeholderValues := map[string]struct{}{
		"change-me":                                  {},
		"changeme":                                   {},
		"dev-im-ai-internal-token":                   {},
		"dev-notify-internal-token":                  {},
		"local-dev-secret-change-me-32-chars":        {},
		"replace-me":                                 {},
		"replace-with-at-least-32-random-characters": {},
		"your-password":                              {},
		"your-secret-key":                            {},
	}
	if _, ok := placeholderValues[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "placeholder") ||
		strings.Contains(normalized, "replace-with") ||
		strings.HasPrefix(normalized, "your-")
}

func normalizeSecretValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
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
