// Package config provides 12-factor, environment-only configuration for the
// auth service. It intentionally keeps the same struct shape, global Cfg
// variable, helper methods, and environment variable names as the monolith's
// config package so that code copied from the monolith keeps working
// unchanged and docker-compose environments stay uniform.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	envsecret "github.com/go-admin-kit/services/shared/pkg/envsecret"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	App            AppCfg
	Database       DatabaseConfig
	Redis          RedisConfig
	JWT            JWTConfig
	CORS           CORSConfig
	Logger         LoggerConfig
	OAuth          OAuthConfig
	Security       SecurityConfig
	Observability  ObservabilityConfig
	NATS           NATSConfig
	Retention      RetentionConfig
	Notify         NotifyConfig
	SecurityDetect SecurityDetectConfig
	Webhook        WebhookConfig
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
	SearchPath             string
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

// NotifyConfig configures the shared notifyclient (in-console alerts) used by
// the security event detector. Empty values disable notifications.
type NotifyConfig struct {
	APIBase string
	Token   string
}

// SecurityDetectConfig tunes the audit anomaly detector thresholds.
type SecurityDetectConfig struct {
	WriteThreshold      int
	PermissionThreshold int
	FailureThreshold    int
}

// WebhookConfig controls outbound webhook secret encryption and delivery.
type WebhookConfig struct {
	EncryptionKeyBase64   string
	KeyID                 string
	AllowHTTP             bool
	AllowPrivate          bool
	ScanIntervalSeconds   int
	BatchSize             int
	MaxAttempts           int
	RequestTimeoutSeconds int
}

type NATSConfig struct {
	// URL is the NATS server URL; empty disables event publishing.
	URL string
}

// RetentionConfig 控制日志保留策略。
type RetentionConfig struct {
	// LogRetentionDays 操作/登录日志保留天数；<=0（默认）关闭自动清理——
	// 绝不隐式删数据。业务审计日志（audit_logs）刻意不在自动清理范围，
	// 它是合规取证面，要清只能走显式运维操作。
	LogRetentionDays int
	// LogRetentionScanIntervalSeconds 清理扫描周期（秒），默认一天。
	LogRetentionScanIntervalSeconds int
}

var Cfg Config

// Defaults returns the local-development configuration. Values match the
// monolith's configs/config.yaml so both services behave identically against
// the shared Postgres/Redis, except App.Port which defaults to 8082.
func Defaults() Config {
	return Config{
		App: AppCfg{
			Name:    "go-admin-kit-audit",
			Version: "1.0.0",
			Env:     "development",
			Port:    8085,
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
				ServiceName:  "go-admin-kit-audit",
				Environment:  "development",
				OTLPEndpoint: "localhost:4317",
				SampleRatio:  1.0,
			},
		},
		NATS: NATSConfig{URL: ""},
		Retention: RetentionConfig{
			LogRetentionDays:                0,
			LogRetentionScanIntervalSeconds: 86400,
		},
		Webhook: WebhookConfig{
			KeyID: "current", ScanIntervalSeconds: 2, BatchSize: 50,
			MaxAttempts: 5, RequestTimeoutSeconds: 10,
		},
	}
}

// Load fills the package-level Cfg from environment variables layered over
// Defaults. Env var names match the monolith exactly.
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
	config.Database.SearchPath = getEnvString("DB_SEARCH_PATH", config.Database.SearchPath)
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
	config.Notify.APIBase = getEnvString("NOTIFY_API_BASE", config.Notify.APIBase)
	config.Notify.Token = getSecretString("NOTIFY_INTERNAL_TOKEN", config.Notify.Token)
	config.SecurityDetect.WriteThreshold = getEnvInt("SECURITY_DETECT_WRITE_THRESHOLD", 20)
	config.SecurityDetect.PermissionThreshold = getEnvInt("SECURITY_DETECT_PERMISSION_THRESHOLD", 5)
	config.SecurityDetect.FailureThreshold = getEnvInt("SECURITY_DETECT_FAILURE_THRESHOLD", 10)
	config.Webhook.EncryptionKeyBase64 = getSecretString("WEBHOOK_ENCRYPTION_KEY", config.Webhook.EncryptionKeyBase64)
	config.Webhook.KeyID = getEnvString("WEBHOOK_KEY_ID", config.Webhook.KeyID)
	config.Webhook.AllowHTTP = getEnvBool("WEBHOOK_ALLOW_HTTP", false)
	config.Webhook.AllowPrivate = getEnvBool("WEBHOOK_ALLOW_PRIVATE", false)
	config.Webhook.ScanIntervalSeconds = getEnvInt("WEBHOOK_SCAN_INTERVAL_SECONDS", config.Webhook.ScanIntervalSeconds)
	config.Webhook.BatchSize = getEnvInt("WEBHOOK_BATCH_SIZE", config.Webhook.BatchSize)
	config.Webhook.MaxAttempts = getEnvInt("WEBHOOK_MAX_ATTEMPTS", config.Webhook.MaxAttempts)
	config.Webhook.RequestTimeoutSeconds = getEnvInt("WEBHOOK_REQUEST_TIMEOUT_SECONDS", config.Webhook.RequestTimeoutSeconds)
	config.Retention.LogRetentionDays = getEnvInt("AUDIT_LOG_RETENTION_DAYS", config.Retention.LogRetentionDays)
	config.Retention.LogRetentionScanIntervalSeconds = getEnvInt("AUDIT_LOG_RETENTION_SCAN_INTERVAL_SECONDS", config.Retention.LogRetentionScanIntervalSeconds)
}

// WebhookEncryptionKey decodes the AES-256 key used for subscription secrets.
func WebhookEncryptionKey(cfg Config) ([]byte, error) {
	value := strings.TrimSpace(cfg.Webhook.EncryptionKeyBase64)
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("WEBHOOK_ENCRYPTION_KEY must be base64 for exactly 32 bytes")
	}
	return decoded, nil
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
	if _, err := WebhookEncryptionKey(cfg); err != nil {
		return err
	}
	if isProductionEnv(cfg.App.Env) {
		// Collect every secret problem before failing so an operator fixes the
		// whole set in one pass instead of one restart per issue.
		issues := make([]string, 0, 3)
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
		if isWeakCredential(cfg.Database.Password) {
			issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if isWeakCredential(cfg.Redis.Password) {
			issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if len(issues) > 0 {
			return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
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

// GetDSN returns the database connection string (same shape as the monolith).
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
	// Phase 2A：schema-per-service——search_path=audit_svc,public（own 表优先，共享表兜底 public）
	if sp := strings.TrimSpace(c.SearchPath); sp != "" {
		dsn += " search_path=" + sp
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

// GetRedisAddr returns the Redis address.
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
