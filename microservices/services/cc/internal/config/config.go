// Package config provides environment-only configuration for cc-service,
// following the im-service convention (lightweight experimental services do
// not carry the full config framework of the core services).
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
	AppPort    string
	AppEnv     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	JWTSecret  string

	// WebhookSecret verifies X-CC-Signature from freeswitch-cc control-api
	// (HMAC-SHA256 over the raw body). Empty disables verification — dev only.
	WebhookSecret string
	// FSAPIBase is the freeswitch-cc control-api base URL, e.g.
	// http://control-api:8090. Empty disables the status/test proxies.
	FSAPIBase string
	// FSAPIToken is sent as X-CC-Token to control-api.
	FSAPIToken string
	// RecordingPublicBase rewrites recording paths for playback links.
	RecordingPublicBase string
	// CDRAuth mod_json_cdr Basic 凭据 "user:pass"（与 FS 侧 CC_CDR_CRED 一致）
	CDRAuth string

	// CRMAPIBase 是 crm-service 内网基址（如 http://go-admin-kit-crm:8091）；
	// CRMInternalToken 是内部端点共享密钥（X-Internal-Token）。任一为空=不接 CRM：
	// 号码识客只返回 cc 侧数据，挂机不写 CRM 跟进。
	CRMAPIBase       string
	CRMInternalToken string
	// InternalToken 给 notify-service 调语音通知内部端点。空=关闭。
	InternalToken string

	// TTS（OpenAI 兼容 /v1/audio/speech）预生成提示音。TTSURL 为空=未配置。
	TTSURL   string
	TTSKey   string
	TTSModel string
	TTSVoice string

	// ASR（OpenAI 兼容 /v1/audio/transcriptions）录音转写。ASRURL 为空=未配置。
	ASRURL   string
	ASRKey   string
	ASRModel string

	// 余额预警（转售账务）：可用额（余额+透支额度）≤ WarnFen 的租户按小时扫描，
	// 站内信通知平台管理员（notify internal send，ref 去重=每租户每天一条）。
	// NotifyAPIBase 为空=不预警。
	NotifyAPIBase       string
	NotifyInternalToken string
	BalanceWarnFen      int64  // 阈值（分），默认 1000（10 元）
	BalanceWarnUserID   uint64 // 收件人 user_id，默认 1（平台管理员）
	// AutoFreeze 欠费自动冻结（可用额≤0 自动冻结+停用线路），默认关。
	AutoFreeze bool
	// WebhookLogKeepDays cc_webhook_logs 留档天数（每日清理），默认 30。
	WebhookLogKeepDays int
	// TranscribeConcurrency 录音转写并发上限（后台 worker），默认 1。转写走
	// io.Pipe 流式转发不缓冲录音，但 ASR 配额与本进程 CPU 仍要收口。
	TranscribeConcurrency int
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
		AppPort:    getenv("APP_PORT", "8089"),
		AppEnv:     getenv("APP_ENV", "development"),
		DBHost:     getenv("DB_HOST", "127.0.0.1"),
		DBPort:     getenv("DB_PORT", "5432"),
		DBUser:     getenv("DB_USER", "postgres"),
		DBPassword: getSecret("DB_PASSWORD", "123456"),
		DBName:     getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:  getenv("DB_SSLMODE", "disable"),
		JWTSecret:  getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),

		WebhookSecret:       getSecret("CC_WEBHOOK_SECRET", ""),
		FSAPIBase:           strings.TrimRight(getenv("CC_FS_API_BASE", ""), "/"),
		FSAPIToken:          getSecret("CC_FS_TOKEN", ""),
		RecordingPublicBase: getenv("CC_RECORDING_PUBLIC_BASE", ""),
		CDRAuth:             getenv("CC_CDR_AUTH", ""),

		CRMAPIBase:       strings.TrimRight(getenv("CC_CRM_API_BASE", ""), "/"),
		CRMInternalToken: getSecret("CC_CRM_INTERNAL_TOKEN", ""),
		InternalToken:    getSecret("CC_INTERNAL_TOKEN", ""),

		TTSURL:   strings.TrimRight(getenv("CC_TTS_URL", ""), "/"),
		TTSKey:   getenv("CC_TTS_KEY", ""),
		TTSModel: getenv("CC_TTS_MODEL", "tts-1"),
		TTSVoice: getenv("CC_TTS_VOICE", "alloy"),

		ASRURL:   strings.TrimRight(getenv("CC_ASR_URL", ""), "/"),
		ASRKey:   getenv("CC_ASR_KEY", ""),
		ASRModel: getenv("CC_ASR_MODEL", "whisper-1"),

		NotifyAPIBase:       strings.TrimRight(getenv("CC_NOTIFY_API_BASE", ""), "/"),
		NotifyInternalToken: getSecret("CC_NOTIFY_INTERNAL_TOKEN", ""),
		BalanceWarnFen:      getenvInt64("CC_BALANCE_WARN_FEN", 1000),
		BalanceWarnUserID:   uint64(getenvInt64("CC_BALANCE_WARN_USER", 1)),
		AutoFreeze:          getenv("CC_AUTO_FREEZE", "false") == "true",
		WebhookLogKeepDays:  int(getenvInt64("CC_WEBHOOK_LOG_KEEP_DAYS", 30)),

		TranscribeConcurrency: int(getenvInt64("CC_TRANSCRIBE_CONCURRENCY", 1)),
	}
}

// validate 只硬拦"没有降级语义"的两项：JWT_SECRET 与 DB_PASSWORD。密钥不对
// 就没有能安全退化的行为，只能拒绝启动。口径与 auth、monitor 等核心服务一致
// （占位符黑名单 + 弱值黑名单 + 32 位门槛）。轻量服务不引 shared 依赖
// （Docker 构建上下文只含本服务目录），故为本地副本。
//
// 其余鉴权密钥一律不阻断启动，改为在使用点 fail closed（见 sanitize）：
// 阻断启动会把"少配一个可选 token"放大成整个服务起不来，代价远大于收益。
//
// TTS/ASR 的 key 不在校验范围：事实源是 system_settings（见 speech.go），
// env 只是兜底，控制台配好时 env 为空是正常状态。
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

// sanitize 生产环境下把"配了等于没配"的密钥（空/占位符/弱值）显式归零，并返回
// 需要打 WARNING 的说明。归零后所有使用点既有的"空=关闭该能力"分支自然 fail
// closed：internal 端点 503、外呼客户端不启用，绝不拿开发占位符当真凭据用。
//
// WebhookSecret/CDRAuth 是例外——它们空值原本是 fail open（跳过验签/不校验
// Basic），所以归零之外还要靠 ProductionMode() 让端点直接拒绝请求。
func sanitize(c *Config) []string {
	if !isProductionEnv(c.AppEnv) {
		return nil
	}
	warnings := make([]string, 0)
	blank := func(name string, field *string, consequence string) {
		if isWeakCredential(*field) {
			if strings.TrimSpace(*field) != "" {
				warnings = append(warnings, name+" is a placeholder or weak value; ignoring it — "+consequence)
			} else {
				warnings = append(warnings, name+" is not set — "+consequence)
			}
			*field = ""
		}
	}

	blank("CC_WEBHOOK_SECRET", &c.WebhookSecret, "POST /api/v1/cc/webhook is refused (503) until a real secret is configured")
	blank("CC_CDR_AUTH", &c.CDRAuth, "POST /api/v1/cc/cdr is refused (503) until real credentials are configured")
	if c.FSAPIBase != "" {
		blank("CC_FS_TOKEN", &c.FSAPIToken, "freeswitch control-api calls stay unauthenticated")
	}
	if c.CRMAPIBase != "" {
		blank("CC_CRM_INTERNAL_TOKEN", &c.CRMInternalToken, "CRM integration is disabled")
	}
	if c.NotifyAPIBase != "" {
		blank("CC_NOTIFY_INTERNAL_TOKEN", &c.NotifyInternalToken, "balance-warning inbox push is disabled")
	}
	if strings.TrimSpace(c.InternalToken) != "" {
		blank("CC_INTERNAL_TOKEN", &c.InternalToken, "POST /api/v1/cc/internal/voice-notify is refused (503)")
	}
	return warnings
}

// ProductionMode 供使用点判断是否要 fail closed（生产缺密钥 = 关端点，
// 开发环境保持原有的"空密钥即免鉴权"便利）。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) } // 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

func getenvInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
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
