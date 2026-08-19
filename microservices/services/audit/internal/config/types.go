package config

import (
	"fmt"
	"strings"
	"time"
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
