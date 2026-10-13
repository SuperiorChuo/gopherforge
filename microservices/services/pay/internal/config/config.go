// Package config provides environment-only configuration for pay-service
// (lightweight experimental domain service, same convention as ticket/crm).
package config

import (
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

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
	// InternalToken 空=禁用 /internal 端点（照 CRM/BPM 先例）
	InternalToken string
	// OrderTTL 建单默认过期时长
	OrderTTL time.Duration
	// CallbackMaxAttempts 业务回调最大重试次数（指数退避 30s 起）
	CallbackMaxAttempts int
	// CallbackTargets 业务回调注册表：PAY_CALLBACK_<BIZTYPE>=URL（大写 BIZTYPE）
	CallbackTargets map[string]string
	// CallbackToken 回调业务方时携带的 X-Internal-Token
	CallbackToken string
	// Phase 2D：NATS 审计事件发布（audit.log.*）。
	NATSURL string
	// DemoMode 演示假数据总闸：开 = 注册 mock 渠道 + 挂载收银台模拟支付端点。
	// 非生产默认开（本地联调 / CI / pay-smoke 依赖 mock 渠道跑通全链路），
	// 生产默认关——收银台是匿名页面，模拟支付端点等于"谁拿到链接谁能把订单
	// 标成已付"。公开演示站需要时用 PAY_DEMO_MODE=true 显式打开。
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
	c := Config{
		AppPort:             getenv("APP_PORT", "8097"),
		AppEnv:              getenv("APP_ENV", "development"),
		DBHost:              getenv("DB_HOST", "127.0.0.1"),
		DBPort:              getenv("DB_PORT", "5432"),
		DBUser:              getenv("DB_USER", "postgres"),
		DBPassword:          getSecret("DB_PASSWORD", "123456"),
		DBName:              getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:           getenv("DB_SSLMODE", "disable"),
		JWTSecret:           getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		InternalToken:       getSecret("PAY_INTERNAL_TOKEN", ""),
		CallbackToken:       getSecret("PAY_CALLBACK_TOKEN", ""),
		NATSURL:             getenv("NATS_URL", ""),
		OrderTTL:            30 * time.Minute,
		CallbackMaxAttempts: 8,
		CallbackTargets:     map[string]string{},
	}
	c.DemoMode = demoModeEnabled("PAY_DEMO_MODE", c.AppEnv)
	if v, err := strconv.Atoi(getenv("PAY_ORDER_TTL_MINUTES", "30")); err == nil && v > 0 {
		c.OrderTTL = time.Duration(v) * time.Minute
	}
	if v, err := strconv.Atoi(getenv("PAY_CALLBACK_MAX_ATTEMPTS", "8")); err == nil && v > 0 {
		c.CallbackMaxAttempts = v
	}
	// 回调目标按 biz_type 注册：PAY_CALLBACK_CC_RECHARGE=http://... → key "cc_recharge"
	for _, kv := range os.Environ() {
		k, v, found := strings.Cut(kv, "=")
		if !found || !strings.HasPrefix(k, "PAY_CALLBACK_") || v == "" {
			continue
		}
		switch k {
		case "PAY_CALLBACK_TOKEN", "PAY_CALLBACK_MAX_ATTEMPTS":
			continue
		}
		bizType := strings.ToLower(strings.TrimPrefix(k, "PAY_CALLBACK_"))
		c.CallbackTargets[bizType] = strings.TrimSpace(v)
	}
	return c
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
	gate("PAY_INTERNAL_TOKEN", &c.InternalToken, "internal endpoints reject all callers (503)")
	notice("PAY_CALLBACK_TOKEN", c.CallbackToken, "business callbacks are sent without an authentication header")
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
}

// 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

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
