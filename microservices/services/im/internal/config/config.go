package config

import (
	"fmt"
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	AppPort     string
	AppEnv      string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	DBSSLMode   string
	JWTSecret   string
	CORSOrigins []string
	UploadDir   string

	// 附件存储："local"（默认）或 "minio"。
	// MinIO 设置复用栈级 UPLOAD_MINIO_* 变量。
	StorageType    string
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool

	// NATSURL 开启跨实例 WS 扇出；为空则仅进程内分发。
	NATSURL string

	// RetentionDays 在会话关闭满该天数后硬删除（含消息和
	// 附件对象）。0 表示永久保留。
	RetentionDays int

	// IdleCloseMinutes 在无消息持续该时长时自动关闭已分配/排队中的会话
	// （reason=timeout）。0 表示禁用。
	IdleCloseMinutes int

	// SensitiveWords 是逗号分隔的屏蔽词列表；文本消息中命中的子串
	// 在持久化前会被 * 遮盖。空值表示禁用。
	SensitiveWords []string

	// AI / bot（M4）— OpenAI 兼容；key 为空则使用本地 stub。
	AIEnabled      bool
	AIBaseURL      string
	AIAPIKey       string
	AIModel        string
	AISystemPrompt string
	AITimeout      time.Duration

	// AI 服务向量知识库检索（M1.5）。token 为空则禁用向量路径。
	AIServiceURL    string
	AIInternalToken string

	// 通知中心（站内信）。token 为空则禁用转人工收件箱推送。
	NotifyAPIBase       string
	NotifyInternalToken string

	// AuthAPIBase 是 auth-service 内部地址。IM 用它校验坐席访问令牌的
	// 有效性/吊销状态及当前 IM 权限（/internal/verify）；配置缺失时坐席
	// 路径 fail closed，不影响访客链路。
	AuthAPIBase string
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
	timeoutSec := EnvInt("AI_TIMEOUT_SEC", 45)
	return Config{
		AppPort:    getenv("APP_PORT", "8088"),
		AppEnv:     getenv("APP_ENV", "development"),
		DBHost:     getenv("DB_HOST", "127.0.0.1"),
		DBPort:     getenv("DB_PORT", "5432"),
		DBUser:     getenv("DB_USER", "postgres"),
		DBPassword: getSecret("DB_PASSWORD", "123456"),
		DBName:     getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:  getenv("DB_SSLMODE", "disable"),
		JWTSecret:  getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		UploadDir:  getenv("IM_UPLOAD_DIR", "./uploads"),

		StorageType:    strings.ToLower(getenv("IM_STORAGE_TYPE", "local")),
		MinIOEndpoint:  getenv("UPLOAD_MINIO_ENDPOINT", ""),
		MinIOAccessKey: getSecret("UPLOAD_MINIO_ACCESS_KEY", ""),
		MinIOSecretKey: getSecret("UPLOAD_MINIO_SECRET_KEY", ""),
		MinIOBucket:    getenv("UPLOAD_MINIO_BUCKET", "go-admin-kit"),
		MinIOUseSSL:    EnvBool("UPLOAD_MINIO_USE_SSL", false),

		NATSURL: getenv("NATS_URL", ""),

		RetentionDays:    EnvInt("IM_RETENTION_DAYS", 180),
		IdleCloseMinutes: EnvInt("IM_IDLE_CLOSE_MINUTES", 30),
		SensitiveWords:   splitCSV(getenv("IM_SENSITIVE_WORDS", "")),
		CORSOrigins:      splitCSV(getenv("CORS_ALLOW_ORIGINS", "http://localhost:8000,http://localhost:3000,http://127.0.0.1:3000")),

		// 默认开启，使 bot_serving 路径在 stub 离线时也能工作。
		AIEnabled:      EnvBool("AI_ENABLED", true),
		AIBaseURL:      getenv("AI_BASE_URL", getenv("OPENAI_BASE_URL", "https://api.openai.com")),
		AIAPIKey:       getSecret("AI_API_KEY", getSecret("OPENAI_API_KEY", "")),
		AIModel:        getenv("AI_MODEL", getenv("OPENAI_MODEL", "gpt-4o-mini")),
		AISystemPrompt: getenv("AI_SYSTEM_PROMPT", ""),
		AITimeout:      time.Duration(timeoutSec) * time.Second,

		AIServiceURL:    getenv("AI_SERVICE_URL", "http://ai-service:8087"),
		AIInternalToken: getSecret("AI_INTERNAL_TOKEN", ""),

		NotifyAPIBase:       getenv("NOTIFY_API_BASE", "http://go-admin-kit-notify:8095"),
		NotifyInternalToken: getSecret("NOTIFY_INTERNAL_TOKEN", ""),

		AuthAPIBase: getenv("AUTH_API_BASE", "http://go-admin-kit-auth:8082"),
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
	notice("AI_INTERNAL_TOKEN", c.AIInternalToken, "vector KB search may be rejected by ai-service")
	notice("NOTIFY_INTERNAL_TOKEN", c.NotifyInternalToken, "inbox push may be rejected by notify-service")
	// MinIO 凭据只在附件真的走 MinIO 时才有意义。这里刻意只告警不归零：
	// 抹掉会让 NewMinIO 连接失败并静默回落本地盘，已上传的附件从此读不到——
	// 弱凭据要在 infra 侧换掉，不该由 im 单方面把存储降级。
	if c.StorageType == "minio" {
		notice("UPLOAD_MINIO_ACCESS_KEY", c.MinIOAccessKey, "attachment storage is using well-known default credentials")
		notice("UPLOAD_MINIO_SECRET_KEY", c.MinIOSecretKey, "attachment storage is using well-known default credentials")
	}
	return warnings
}

// ProductionMode 供使用点判断是否要 fail closed。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) }

// 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
// 原 IM 特有的 dev-im-ai-internal-token / dev-notify-internal-token 占位符已纳入共享包统一维护。
var (
	isProductionEnv    = secretstrength.IsProductionEnv
	isStrongSecret     = secretstrength.IsStrongSecret
	isWeakCredential   = secretstrength.IsWeakCredential
	isPlaceholderValue = secretstrength.IsPlaceholderValue
)

func (c Config) DSN() string {
	dsn := "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=Asia/Shanghai"
	// Phase 2B：schema-per-service——search_path 读 DB_SEARCH_PATH env（空则不追加）
	if sp := os.Getenv("DB_SEARCH_PATH"); sp != "" {
		dsn += " search_path=" + sp
	}
	return dsn
}

func getSecret(key, fallback string) string {
	return envsecret.Get(key, fallback)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func EnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func EnvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return def
	}
}
