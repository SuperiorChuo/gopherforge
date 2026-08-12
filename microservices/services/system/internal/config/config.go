// Package config 提供 12-factor、仅环境变量驱动的配置，供
// auth 服务使用。它刻意保持与单体 config 包相同的结构体形态、全局 Cfg
// 变量、辅助方法与环境变量名称，使从单体拷贝的代码无需改动即可继续
// 工作，并让 docker-compose 环境保持一致。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	envsecret "github.com/go-admin-kit/services/shared/pkg/envsecret"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	App           AppCfg
	Database      DatabaseConfig
	Redis         RedisConfig
	JWT           JWTConfig
	CORS          CORSConfig
	Logger        LoggerConfig
	OAuth         OAuthConfig
	Security      SecurityConfig
	Observability ObservabilityConfig
	Notification  NotificationConfig
	NATS          NATSConfig
	Codegen       CodegenConfig
	InternalToken string
}

type CodegenConfig struct {
	WriteEnabled bool
	RepoRoot     string
}

type NotificationConfig struct {
	Email EmailConfig
}

type EmailConfig struct {
	Enabled         bool
	SMTPHost        string
	SMTPPort        int
	Username        string
	Password        string
	Sender          string
	AlertReceiver   string
	AlertReceivers  []string
	SubjectTemplate string
	BodyTemplate    string
	RecipientGroups map[string][]string
	UseTLS          bool
	StartTLS        bool
}

type AppCfg struct {
	Name    string
	Version string
	Env     string
	Port    int
}

type DatabaseConfig struct {
	Driver                 string
	Host                   string
	Port                   int
	User                   string
	Password               string
	DBName                 string
	SSLMode                string
	MaxIdleConns           int
	MaxOpenConns           int
	ConnMaxLifetimeSeconds int
	ConnMaxIdleTimeSeconds int
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

type JWTConfig struct {
	Secret               string
	AccessTokenExpire    int
	RefreshTokenExpire   int
	RefreshTokenRotation bool
	Issuer               string
}

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

type LoggerConfig struct {
	Level      string
	FilePath   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
}

type OAuthConfig struct {
	Github OAuthProviderConfig
	Wechat OAuthProviderConfig
}

type OAuthProviderConfig struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

func (c OAuthProviderConfig) Ready() bool {
	return c.Enabled &&
		oauthConfigValueReady(c.ClientID) &&
		oauthConfigValueReady(c.ClientSecret) &&
		oauthConfigValueReady(c.RedirectURI)
}

type SecurityConfig struct {
	TrustedProxies       []string
	PasswordMaxAgeDays   int
	PasswordHistoryCount int
	Headers              SecurityHeaders
	RateLimit            RateLimitConfig
	LoginLimit           LoginLimitConfig
	DefaultAdmin         DefaultAdminConfig
}

type SecurityHeaders struct {
	Enabled bool
	HSTS    bool
}

type RateLimitConfig struct {
	Enabled       bool
	WindowSeconds int
	MaxRequests   int
}

type LoginLimitConfig struct {
	Enabled       bool
	WindowMinutes int
	MaxFailures   int
	LockMinutes   int
}

type DefaultAdminConfig struct {
	WarnDefaultPassword bool
	ForceChangePassword bool
	DefaultUsername     string
}

type ObservabilityConfig struct {
	RequestIDHeader string
	Tracing         TracingConfig
}

type TracingConfig struct {
	Enabled      bool
	ServiceName  string
	Environment  string
	OTLPEndpoint string
	SampleRatio  float64
}

type NATSConfig struct {
	// URL 是 NATS 服务器地址；为空时禁用事件发布。
	URL string
}

var Cfg Config

// Defaults 返回本地开发配置。取值与单体的 configs/config.yaml 一致，
// 使两个服务面对共享的 Postgres/Redis 时行为完全相同，
// 唯一例外是 App.Port 默认为 8082。
func Defaults() Config {
	return Config{
		App: AppCfg{
			Name:    "go-admin-kit-system",
			Version: "1.0.0",
			Env:     "development",
			Port:    8084,
		},
		Database: DatabaseConfig{
			Driver:                 "postgres",
			Host:                   "localhost",
			Port:                   5432,
			User:                   "postgres",
			Password:               "123456",
			DBName:                 "go_admin_kit",
			SSLMode:                "disable",
			MaxIdleConns:           5,
			MaxOpenConns:           10,
			ConnMaxLifetimeSeconds: 300,
			ConnMaxIdleTimeSeconds: 180,
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
			PoolSize: 100,
		},
		JWT: JWTConfig{
			Secret:               "your-secret-key",
			AccessTokenExpire:    3600,
			RefreshTokenExpire:   86400,
			RefreshTokenRotation: true,
			Issuer:               "go-admin-kit",
		},
		CORS: CORSConfig{
			AllowOrigins: []string{
				"http://127.0.0.1:3000",
				"http://localhost:3000",
				"http://127.0.0.1:3001",
				"http://localhost:3001",
				"http://127.0.0.1:3002",
				"http://localhost:3002",
			},
			AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
			AllowHeaders: []string{
				"Origin",
				"Content-Type",
				"Authorization",
				"X-Requested-With",
				"Accept",
				"X-Token",
				"X-Request-ID",
			},
			ExposeHeaders: []string{
				"Content-Length",
				"Content-Type",
				"Authorization",
				"X-Request-ID",
			},
			AllowCredentials: true,
			MaxAge:           12,
		},
		Logger: LoggerConfig{
			Level:      "info",
			FilePath:   "./logs/app.log",
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     30,
		},
		OAuth: OAuthConfig{
			Github: OAuthProviderConfig{Enabled: false},
			Wechat: OAuthProviderConfig{Enabled: false},
		},
		Security: SecurityConfig{
			TrustedProxies:       []string{"127.0.0.1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			PasswordMaxAgeDays:   90,
			PasswordHistoryCount: 5,
			Headers:              SecurityHeaders{Enabled: true, HSTS: false},
			RateLimit:            RateLimitConfig{Enabled: true, WindowSeconds: 1, MaxRequests: 100},
			LoginLimit:           LoginLimitConfig{Enabled: true, WindowMinutes: 15, MaxFailures: 5, LockMinutes: 30},
			DefaultAdmin: DefaultAdminConfig{
				WarnDefaultPassword: true,
				ForceChangePassword: false,
				DefaultUsername:     "admin",
			},
		},
		Observability: ObservabilityConfig{
			RequestIDHeader: "X-Request-ID",
			Tracing: TracingConfig{
				Enabled:      false,
				ServiceName:  "go-admin-kit-system",
				Environment:  "development",
				OTLPEndpoint: "localhost:4317",
				SampleRatio:  1.0,
			},
		},
		NATS: NATSConfig{URL: ""},
		Codegen: CodegenConfig{
			WriteEnabled: false,
			RepoRoot:     "",
		},
	}
}

// Load 用叠加在 Defaults 之上的环境变量填充包级 Cfg。
// 环境变量名称与单体完全一致。
func Load() error {
	cfg := Defaults()
	applyEnv(&cfg)
	if err := validate(cfg); err != nil {
		return err
	}
	Cfg = cfg
	return nil
}

func applyEnv(config *Config) {
	config.App.Env = getEnvString("APP_ENV", config.App.Env)
	config.App.Port = getEnvInt("APP_PORT", config.App.Port)

	config.Database.Host = getEnvString("DB_HOST", config.Database.Host)
	config.Database.Port = getEnvInt("DB_PORT", config.Database.Port)
	config.Database.User = getEnvString("DB_USER", config.Database.User)
	// 敏感项走 envsecret：优先 /run/secrets，再环境变量。
	config.Database.Password = getSecretString("DB_PASSWORD", config.Database.Password)
	config.Database.DBName = getEnvString("DB_NAME", config.Database.DBName)
	config.Database.SSLMode = getEnvString("DB_SSLMODE", config.Database.SSLMode)
	config.Database.MaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", config.Database.MaxIdleConns)
	config.Database.MaxOpenConns = getEnvInt("DB_MAX_OPEN_CONNS", config.Database.MaxOpenConns)
	config.Database.ConnMaxLifetimeSeconds = getEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", config.Database.ConnMaxLifetimeSeconds)
	config.Database.ConnMaxIdleTimeSeconds = getEnvInt("DB_CONN_MAX_IDLE_TIME_SECONDS", config.Database.ConnMaxIdleTimeSeconds)

	config.Redis.Host = getEnvString("REDIS_HOST", config.Redis.Host)
	config.Redis.Port = getEnvInt("REDIS_PORT", config.Redis.Port)
	config.Redis.Password = getSecretString("REDIS_PASSWORD", config.Redis.Password)
	config.Redis.DB = getEnvInt("REDIS_DB", config.Redis.DB)

	config.JWT.Secret = getSecretString("JWT_SECRET", config.JWT.Secret)
	config.JWT.RefreshTokenRotation = getEnvBool("JWT_REFRESH_TOKEN_ROTATION", config.JWT.RefreshTokenRotation)

	config.CORS.AllowOrigins = getEnvStringSlice("CORS_ALLOW_ORIGINS", config.CORS.AllowOrigins)
	config.CORS.AllowCredentials = getEnvBool("CORS_ALLOW_CREDENTIALS", config.CORS.AllowCredentials)

	config.Security.TrustedProxies = getEnvStringSlice("TRUSTED_PROXIES", config.Security.TrustedProxies)
	config.Security.PasswordMaxAgeDays = getEnvInt("PASSWORD_MAX_AGE_DAYS", config.Security.PasswordMaxAgeDays)
	config.Security.PasswordHistoryCount = getEnvInt("PASSWORD_HISTORY_COUNT", config.Security.PasswordHistoryCount)
	config.Security.Headers.Enabled = getEnvBool("SECURITY_HEADERS_ENABLED", config.Security.Headers.Enabled)
	config.Security.Headers.HSTS = getEnvBool("SECURITY_HSTS_ENABLED", config.Security.Headers.HSTS)
	config.Security.RateLimit.Enabled = getEnvBool("RATE_LIMIT_ENABLED", config.Security.RateLimit.Enabled)
	config.Security.RateLimit.WindowSeconds = getEnvInt("RATE_LIMIT_WINDOW_SECONDS", config.Security.RateLimit.WindowSeconds)
	config.Security.RateLimit.MaxRequests = getEnvInt("RATE_LIMIT_MAX_REQUESTS", config.Security.RateLimit.MaxRequests)
	config.Security.LoginLimit.Enabled = getEnvBool("LOGIN_LIMIT_ENABLED", config.Security.LoginLimit.Enabled)
	config.Security.LoginLimit.WindowMinutes = getEnvInt("LOGIN_LIMIT_WINDOW_MINUTES", config.Security.LoginLimit.WindowMinutes)
	config.Security.LoginLimit.MaxFailures = getEnvInt("LOGIN_LIMIT_MAX_FAILURES", config.Security.LoginLimit.MaxFailures)
	config.Security.LoginLimit.LockMinutes = getEnvInt("LOGIN_LIMIT_LOCK_MINUTES", config.Security.LoginLimit.LockMinutes)
	config.Security.DefaultAdmin.WarnDefaultPassword = getEnvBool("DEFAULT_ADMIN_WARN_DEFAULT_PASSWORD", config.Security.DefaultAdmin.WarnDefaultPassword)
	config.Security.DefaultAdmin.ForceChangePassword = getEnvBool("DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD", config.Security.DefaultAdmin.ForceChangePassword)
	config.Security.DefaultAdmin.DefaultUsername = getEnvString("DEFAULT_ADMIN_USERNAME", config.Security.DefaultAdmin.DefaultUsername)

	config.Observability.RequestIDHeader = getEnvString("REQUEST_ID_HEADER", config.Observability.RequestIDHeader)
	config.Observability.Tracing.Enabled = getEnvBool("TRACING_ENABLED", config.Observability.Tracing.Enabled)
	config.Observability.Tracing.ServiceName = getEnvString("OTEL_SERVICE_NAME", config.Observability.Tracing.ServiceName)
	config.Observability.Tracing.ServiceName = getEnvString("TRACING_SERVICE_NAME", config.Observability.Tracing.ServiceName)
	config.Observability.Tracing.Environment = getEnvString("TRACING_ENVIRONMENT", config.Observability.Tracing.Environment)
	config.Observability.Tracing.OTLPEndpoint = getEnvString("OTEL_EXPORTER_OTLP_ENDPOINT", config.Observability.Tracing.OTLPEndpoint)
	config.Observability.Tracing.OTLPEndpoint = getEnvString("TRACING_OTLP_ENDPOINT", config.Observability.Tracing.OTLPEndpoint)
	config.Observability.Tracing.SampleRatio = getEnvFloat64("TRACING_SAMPLE_RATIO", config.Observability.Tracing.SampleRatio)

	config.OAuth.Github.Enabled = getEnvBool("GITHUB_OAUTH_ENABLED", config.OAuth.Github.Enabled)
	config.OAuth.Github.ClientID = getEnvString("GITHUB_CLIENT_ID", config.OAuth.Github.ClientID)
	config.OAuth.Github.ClientSecret = getSecretString("GITHUB_CLIENT_SECRET", config.OAuth.Github.ClientSecret)
	config.OAuth.Github.RedirectURI = getEnvString("GITHUB_REDIRECT_URI", config.OAuth.Github.RedirectURI)
	config.OAuth.Wechat.Enabled = getEnvBool("WECHAT_OAUTH_ENABLED", config.OAuth.Wechat.Enabled)
	config.OAuth.Wechat.ClientID = getEnvString("WECHAT_CLIENT_ID", config.OAuth.Wechat.ClientID)
	config.OAuth.Wechat.ClientSecret = getSecretString("WECHAT_CLIENT_SECRET", config.OAuth.Wechat.ClientSecret)
	config.OAuth.Wechat.RedirectURI = getEnvString("WECHAT_REDIRECT_URI", config.OAuth.Wechat.RedirectURI)

	config.NATS.URL = getEnvString("NATS_URL", config.NATS.URL)
	config.Codegen.WriteEnabled = getEnvBool("CODEGEN_WRITE_ENABLED", config.Codegen.WriteEnabled)
	config.Codegen.RepoRoot = getEnvString("CODEGEN_REPO_ROOT", config.Codegen.RepoRoot)
	config.InternalToken = getSecretString("SYSTEM_INTERNAL_TOKEN", config.InternalToken)

	config.Notification.Email.Enabled = getEnvBool("EMAIL_NOTIFICATION_ENABLED", config.Notification.Email.Enabled)
	config.Notification.Email.SMTPHost = getEnvString("EMAIL_SMTP_HOST", config.Notification.Email.SMTPHost)
	config.Notification.Email.SMTPPort = getEnvInt("EMAIL_SMTP_PORT", config.Notification.Email.SMTPPort)
	config.Notification.Email.Username = getEnvString("EMAIL_SMTP_USERNAME", config.Notification.Email.Username)
	config.Notification.Email.Password = getSecretString("EMAIL_SMTP_PASSWORD", config.Notification.Email.Password)
	config.Notification.Email.Sender = getEnvString("EMAIL_SENDER", config.Notification.Email.Sender)
	config.Notification.Email.AlertReceiver = getEnvString("EMAIL_ALERT_RECEIVER", config.Notification.Email.AlertReceiver)
	config.Notification.Email.AlertReceivers = getEnvStringSlice("EMAIL_ALERT_RECEIVERS", config.Notification.Email.AlertReceivers)
	config.Notification.Email.SubjectTemplate = getEnvString("EMAIL_SUBJECT_TEMPLATE", config.Notification.Email.SubjectTemplate)
	config.Notification.Email.BodyTemplate = getEnvString("EMAIL_BODY_TEMPLATE", config.Notification.Email.BodyTemplate)
	config.Notification.Email.UseTLS = getEnvBool("EMAIL_USE_TLS", config.Notification.Email.UseTLS)
	config.Notification.Email.StartTLS = getEnvBool("EMAIL_START_TLS", config.Notification.Email.StartTLS)
}

func validate(cfg Config) error {
	if cfg.CORS.AllowCredentials && containsString(cfg.CORS.AllowOrigins, "*") {
		return fmt.Errorf("CORS cannot use '*' when credentials are enabled")
	}
	if cfg.Observability.Tracing.SampleRatio < 0 || cfg.Observability.Tracing.SampleRatio > 1 {
		return fmt.Errorf("TRACING_SAMPLE_RATIO must be between 0 and 1")
	}
	if cfg.Security.PasswordMaxAgeDays < 0 {
		return fmt.Errorf("PASSWORD_MAX_AGE_DAYS must be greater than or equal to 0")
	}
	if cfg.Security.PasswordHistoryCount < 0 {
		return fmt.Errorf("PASSWORD_HISTORY_COUNT must be greater than or equal to 0")
	}
	if cfg.Codegen.WriteEnabled {
		if strings.TrimSpace(cfg.App.Env) != "development" {
			return fmt.Errorf("CODEGEN_WRITE_ENABLED requires APP_ENV=development")
		}
		if err := validateCodegenRepoRoot(cfg.Codegen.RepoRoot); err != nil {
			return err
		}
	}
	if isProductionEnv(cfg.App.Env) {
		// 在失败前收集所有密钥问题，使运维可以一次修完整套配置，
		// 而不是每个问题重启一次。
		issues := make([]string, 0, 4)
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
		if isWeakCredential(cfg.Database.Password) {
			issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if isWeakCredential(cfg.Redis.Password) {
			issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if cfg.InternalToken != "" && isWeakCredential(cfg.InternalToken) {
			issues = append(issues, "SYSTEM_INTERNAL_TOKEN must not use a default, weak, or placeholder value")
		}
		// IsSMTPAuthUnsafe 会重新读取环境本身，在该代码块内看似冗余，
		// 但这是刻意为之：让启动门禁走这个导出的谓词，才能保证这里的检查
		// 与运行时配置层的 fail-closed 守卫始终基于同一份定义。
		if IsSMTPAuthUnsafe(cfg.App.Env, cfg.Notification.Email) {
			issues = append(issues, "EMAIL_SMTP_PASSWORD must not be empty, default, weak, or placeholder while SMTP authentication is configured")
		}
		if len(issues) > 0 {
			return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
		}
	}
	return nil
}

// smtpAuthConfigured 报告邮件通道是否真的会对 SMTP 服务器进行认证。
// 当 Enabled 为 false 时 mailer.SMTPSender.Send 会提前返回，并拒绝空主机；
// 当用户名和密码都为空时，mailer 的 smtpAuth 不会发送任何 AUTH 命令——
// 因此关闭的通道与匿名中继都不携带值得阻塞启动的凭据。只有剩下的一种形态
// （通道开启、主机已设置、使用 AUTH）才会让 EMAIL_SMTP_PASSWORD 流向远程服务器。
func smtpAuthConfigured(email EmailConfig) bool {
	if !email.Enabled || strings.TrimSpace(email.SMTPHost) == "" {
		return false
	}
	return strings.TrimSpace(email.Username) != "" || strings.TrimSpace(email.Password) != ""
}

// IsSMTPAuthUnsafe 报告在 env 环境下使用该邮件配置对 SMTP 服务器认证是否不安全。
// 它是该策略的唯一定义：生产环境之外什么都不拒绝（本地开发保持零配置），
// 从不发送 AUTH 的通道没有可判定的凭据（smtpAuthConfigured），
// 只有剩下那种形态才会把密码交给 isWeakCredential 判定。
//
// 导出给 internal/pkg/runtimeconfig 使用：在 validate() 检查完环境派生设置很久之后，
// 一条 system_settings 记录仍可能把 notification.email 打开——那一层必须
// 对这里拒绝的形态同样采取 fail-closed。刻意共享这一个谓词而非分别导出
// isProductionEnv 与 isWeakCredential，是为了让两道门禁不可能相互偏离，
// 并让原始凭据判断原语远离无关调用方。
func IsSMTPAuthUnsafe(env string, email EmailConfig) bool {
	return isProductionEnv(env) && smtpAuthConfigured(email) && isWeakCredential(email.Password)
}

func validateCodegenRepoRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("CODEGEN_REPO_ROOT is required when repository write is enabled")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("CODEGEN_REPO_ROOT is invalid")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("CODEGEN_REPO_ROOT is invalid")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("CODEGEN_REPO_ROOT is not a directory")
	}
	for _, required := range []string{
		".git",
		filepath.Join("microservices", "services", "system"),
		filepath.Join("microservices", "web"),
	} {
		if _, err := os.Stat(filepath.Join(canonical, required)); err != nil {
			return fmt.Errorf("CODEGEN_REPO_ROOT is missing required repository paths")
		}
	}
	return nil
} // 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}


// getSecretString 读敏感配置：/run/secrets 优先于环境变量（Swarm secrets）。
func getSecretString(key, fallback string) string {
	return envsecret.Get(key, fallback)
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvFloat64(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvStringSlice(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (c SecurityConfig) EffectivePasswordMaxAgeDays() int {
	if c.PasswordMaxAgeDays < 0 {
		return 0
	}
	return c.PasswordMaxAgeDays
}

func (c SecurityConfig) EffectivePasswordHistoryCount() int {
	if c.PasswordHistoryCount < 0 {
		return 0
	}
	return c.PasswordHistoryCount
}

// GetDSN 返回数据库连接字符串（与单体形态一致）。
func (c *DatabaseConfig) GetDSN() string {
	sslMode := strings.TrimSpace(c.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.Host, c.Port, c.User, c.DBName, sslMode)
	if c.Password != "" {
		dsn += " password=" + c.Password
	}
	return dsn
}

func (c DatabaseConfig) EffectiveConnMaxLifetime() time.Duration {
	if c.ConnMaxLifetimeSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.ConnMaxLifetimeSeconds) * time.Second
}

func (c DatabaseConfig) EffectiveConnMaxIdleTime() time.Duration {
	if c.ConnMaxIdleTimeSeconds <= 0 {
		return 3 * time.Minute
	}
	return time.Duration(c.ConnMaxIdleTimeSeconds) * time.Second
}

// GetRedisAddr 返回 Redis 地址。
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
