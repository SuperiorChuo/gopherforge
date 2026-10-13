package config

import (
	"fmt"
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"log"
	"os"
	"strconv"
	"strings"

	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	AppPort        string
	AppEnv         string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	JWTSecret      string
	InternalToken  string
	SystemSMSURL   string
	SystemSMSToken string
	// IdentityAPIBase + IdentityInternalToken：Phase 2C 调 identity owner API
	//（收件人联系方式/租户校验，替代直查共享 users 表）。
	IdentityAPIBase       string
	IdentityInternalToken string
	// CCAPIBase + CCInternalToken：语音通知入队 cc-service。空=voice 通道 skipped。
	CCAPIBase       string
	CCInternalToken string
	// AlertUserID Prometheus 告警站内信收件人 user_id（默认 1=平台管理员）。
	AlertUserID uint64

	// SMTP (optional). Empty host = email channel skipped.
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// SMSMode: empty/off = disabled; stub = log only (M2).
	// stub 只写日志、不发任何短信；真实 provider（阿里云/腾讯云）尚未实现。
	SMSMode string

	// DemoMode 演示假通道总闸：关时 SMSMode=stub 会被降级为"未配置"，
	// 让调度器如实报 skipped，而不是留一条"看起来发出去了"的 stub 记录。
	// 非生产默认开，生产默认关；演示站用 NOTIFY_DEMO_MODE=true 显式打开。
	DemoMode bool

	// DefaultChannels applied when template has empty channels (csv).
	DefaultChannels string

	// OutboxWorkers 外发（email/sms）异步 worker 数；fan-out 的阻塞外呼在这里
	// 并发跑，不再占着 HTTP 请求。缺省 4。
	OutboxWorkers int
	// OutboxQueueSize 外发有界队列容量；满了记日志 + dropped 计数，不静默丢。
	OutboxQueueSize int
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
		AppPort:               getenv("APP_PORT", "8095"),
		AppEnv:                appEnv,
		DBHost:                getenv("DB_HOST", "127.0.0.1"),
		DBPort:                getenv("DB_PORT", "5432"),
		DBUser:                getenv("DB_USER", "postgres"),
		DBPassword:            getSecret("DB_PASSWORD", "123456"),
		DBName:                getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:             getenv("DB_SSLMODE", "disable"),
		JWTSecret:             getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		InternalToken:         getSecret("NOTIFY_INTERNAL_TOKEN", ""),
		SystemSMSURL:          getenv("NOTIFY_SYSTEM_SMS_URL", ""),
		SystemSMSToken:        getSecret("SYSTEM_INTERNAL_TOKEN", ""),
		AlertUserID:           getenvUint("NOTIFY_ALERT_USER", 1),
		IdentityAPIBase:       getenv("IDENTITY_API_BASE", "http://go-admin-kit-identity:8083"),
		IdentityInternalToken: getSecret("INTERNAL_TOKEN", ""),
		CCAPIBase:             strings.TrimRight(getenv("NOTIFY_CC_API_BASE", ""), "/"),
		CCInternalToken:       getSecret("NOTIFY_CC_INTERNAL_TOKEN", ""),

		SMTPHost:     getenv("NOTIFY_SMTP_HOST", ""),
		SMTPPort:     getenv("NOTIFY_SMTP_PORT", "587"),
		SMTPUser:     getenv("NOTIFY_SMTP_USER", ""),
		SMTPPassword: getSecret("NOTIFY_SMTP_PASSWORD", ""),
		SMTPFrom:     getenv("NOTIFY_SMTP_FROM", ""),

		SMSMode:         getenv("NOTIFY_SMS_MODE", ""),
		DemoMode:        demoModeEnabled("NOTIFY_DEMO_MODE", appEnv),
		DefaultChannels: getenv("NOTIFY_DEFAULT_CHANNELS", "inbox"),

		OutboxWorkers:   getenvInt("NOTIFY_OUTBOX_WORKERS", 4, 1, 64),
		OutboxQueueSize: getenvInt("NOTIFY_OUTBOX_QUEUE", 1024, 16, 100000),
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
	gate("NOTIFY_INTERNAL_TOKEN", &c.InternalToken, "internal send/alert endpoints reject all callers (503/401)")
	if c.CCAPIBase != "" {
		gate("NOTIFY_CC_INTERNAL_TOKEN", &c.CCInternalToken, "voice channel stays disabled")
	}
	return warnings
}

// ProductionMode 供使用点判断是否要 fail closed。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) }

// EffectiveSMSMode 实际生效的短信模式：非演示环境下 stub（只写日志、不真发）
// 一律降级为空，让 dispatcher 走"未配置 provider → skipped"分支。告警链路上
// 一条标着 stub 的成功记录，比一条明确的 skipped 更容易让人以为短信发出去了。
func (c Config) EffectiveSMSMode() string {
	if c.SMSMode != "" && !c.DemoMode {
		return ""
	}
	return c.SMSMode
}

// demoModeEnabled 演示假数据开关：显式取值优先（演示站可在生产 env 下打开），
// 未设置时按环境推导——非生产开、生产关。
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

func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
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

// getenvInt 读整数并夹到 [min,max]；非法或越界回退 fallback（配置写错不炸服务）。
func getenvInt(key string, fallback, min, max int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		log.Printf("WARNING: %s=%q invalid (want %d..%d), using %d", key, v, min, max, fallback)
		return fallback
	}
	return n
}

func getenvUint(key string, fallback uint64) uint64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
