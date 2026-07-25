// Package config 提供 bpm-service 的纯环境变量配置（轻量服务约定，
// 不携带核心服务的完整配置框架）。
package config

import (
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
	// InternalToken 校验业务方内网调用 bpm internal 端点的
	// X-Internal-Token；空=禁用 internal 端点（返回 503），不裸奔。
	InternalToken string
	// CallbackToken 终态回调业务方时携带的 X-Internal-Token（业务侧内部
	// 端点鉴权）；空=回调不带鉴权头。
	CallbackToken string
	// NotifyAPIBase + NotifyInternalToken：新待办/抄送/终态站内信；
	// token 空=静默跳过通知（不阻断审批主流程）。
	NotifyAPIBase       string
	NotifyInternalToken string
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
		AppPort:             getenv("APP_PORT", "8096"),
		AppEnv:              getenv("APP_ENV", "development"),
		DBHost:              getenv("DB_HOST", "127.0.0.1"),
		DBPort:              getenv("DB_PORT", "5432"),
		DBUser:              getenv("DB_USER", "postgres"),
		DBPassword:          getenv("DB_PASSWORD", "123456"),
		DBName:              getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:           getenv("DB_SSLMODE", "disable"),
		JWTSecret:           getenv("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		InternalToken:       getenv("BPM_INTERNAL_TOKEN", ""),
		CallbackToken:       getenv("BPM_CALLBACK_TOKEN", ""),
		NotifyAPIBase:       getenv("NOTIFY_API_BASE", ""),
		NotifyInternalToken: getenv("NOTIFY_INTERNAL_TOKEN", ""),
		TimeoutScanInterval: getenvDuration("BPM_TIMEOUT_SCAN_INTERVAL", 5*time.Minute),
	}
}

// validate 只硬拦"没有降级语义"的配置：JWT_SECRET 与 DB_PASSWORD。密钥不对
// 就没有能安全退化的行为，只能拒绝启动。口径与 auth、monitor 等核心服务的
// validateProductionSafety 一致（占位符黑名单 + 弱值黑名单 + 32 位门槛）。
// 轻量服务不引 shared 依赖（Docker 构建上下文只含本服务目录），故为本地副本。
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
//
// 只对"已接线"的可选能力告警：脚手架默认不带站内信服务，NOTIFY_API_BASE 为空
// 即通道整体关闭（notifyclient.Enabled()=false），此时再提 token 缺失是噪声。
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
	notice("BPM_CALLBACK_TOKEN", c.CallbackToken,
		"terminal-state callbacks go out unauthenticated (unset) or carry a token the receiver will reject")
	if strings.TrimSpace(c.NotifyAPIBase) != "" {
		notice("NOTIFY_INTERNAL_TOKEN", c.NotifyInternalToken,
			"inbox push will not reach users: unset disables the channel, a placeholder gets rejected by the notification service")
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
		"123456":       {},
		"admin":        {},
		"changeme":     {},
		"default":      {},
		"demo":         {},
		"development":  {},
		"example":      {},
		"go-admin-kit": {},
		"local":        {},
		"password":     {},
		"redis":        {},
		"root":         {},
		"sample":       {},
		"secret":       {},
		"test":         {},
		"test123":      {},
	}
	_, ok := weakValues[normalized]
	return ok
}

// isPlaceholderValue 覆盖仓库内公开出现过的占位符。dev- 前缀是通配兜底：
// compose 里形如 dev-notify-internal-token 的开发默认值人尽皆知，任何
// dev- 开头的值都不该当生产凭据用（也免得逐个枚举上游各服务的 token 名）。
func isPlaceholderValue(value string) bool {
	normalized := normalizeSecretValue(value)
	if normalized == "" {
		return true
	}
	placeholderValues := map[string]struct{}{
		"change-me":                           {},
		"changeme":                            {},
		"dev-notify-internal-token":           {},
		"local-dev-secret-change-me-32-chars": {},
		"replace-me":                          {},
		"replace-with-at-least-32-random-characters": {},
		"your-password":   {},
		"your-secret-key": {},
	}
	if _, ok := placeholderValues[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "placeholder") ||
		strings.Contains(normalized, "replace-with") ||
		strings.HasPrefix(normalized, "dev-") ||
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
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
