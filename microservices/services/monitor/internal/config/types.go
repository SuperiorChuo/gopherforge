package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	App           AppCfg              `yaml:"app"`
	Database      DatabaseConfig      `yaml:"database"`
	Redis         RedisConfig         `yaml:"redis"`
	JWT           JWTConfig           `yaml:"jwt"`
	CORS          CORSConfig          `yaml:"cors"`
	Logger        LoggerConfig        `yaml:"logger"`
	OAuth         OAuthConfig         `yaml:"oauth"`
	Upload        UploadConfig        `yaml:"upload"`
	Security      SecurityConfig      `yaml:"security"`
	Notification  NotificationConfig  `yaml:"notification"`
	Observability ObservabilityConfig `yaml:"observability"`
	NATS          NATSConfig          `yaml:"nats"`
}

type NATSConfig struct {
	// URL is the NATS server URL; empty disables event consumption.
	URL string `yaml:"url"`
}

type AppCfg struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Env     string `yaml:"env"`
	Port    int    `yaml:"port"`
}

type DatabaseConfig struct {
	Driver                 string `yaml:"driver"`
	Host                   string `yaml:"host"`
	Port                   int    `yaml:"port"`
	User                   string `yaml:"user"`
	Password               string `yaml:"password"`
	DBName                 string `yaml:"dbname"`
	SSLMode                string `yaml:"sslmode"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	ConnMaxLifetimeSeconds int    `yaml:"conn_max_lifetime_seconds"`
	ConnMaxIdleTimeSeconds int    `yaml:"conn_max_idle_time_seconds"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type SecurityConfig struct {
	TrustedProxies       []string           `yaml:"trusted_proxies"`
	PasswordMaxAgeDays   int                `yaml:"password_max_age_days"`
	PasswordHistoryCount int                `yaml:"password_history_count"`
	Headers              SecurityHeaders    `yaml:"headers"`
	RateLimit            RateLimitConfig    `yaml:"rate_limit"`
	LoginLimit           LoginLimitConfig   `yaml:"login_limit"`
	DefaultAdmin         DefaultAdminConfig `yaml:"default_admin"`
}

type SecurityHeaders struct {
	Enabled bool `yaml:"enabled"`
	HSTS    bool `yaml:"hsts"`
}

type RateLimitConfig struct {
	Enabled       bool `yaml:"enabled"`
	WindowSeconds int  `yaml:"window_seconds"`
	MaxRequests   int  `yaml:"max_requests"`
}

type LoginLimitConfig struct {
	Enabled       bool `yaml:"enabled"`
	WindowMinutes int  `yaml:"window_minutes"`
	MaxFailures   int  `yaml:"max_failures"`
	LockMinutes   int  `yaml:"lock_minutes"`
}

type DefaultAdminConfig struct {
	WarnDefaultPassword bool   `yaml:"warn_default_password"`
	ForceChangePassword bool   `yaml:"force_change_password"`
	DefaultUsername     string `yaml:"default_username"`
}

type NotificationConfig struct {
	Email EmailConfig         `yaml:"email"`
	Alert AlertChannelsConfig `yaml:"alert"`
}

// AlertChannelsConfig configures the additional alert notification channels
// of the built-in rule engine: in-console station messages (notify service
// internal endpoint) and WeCom robot webhook. Email lives in EmailConfig. A
// channel whose required value is empty is treated as not configured/skipped.
type AlertChannelsConfig struct {
	StationBaseURL string `yaml:"station_base_url"`
	StationToken   string `yaml:"station_token"`
	WeComWebhook   string `yaml:"wecom_webhook"`
	WebhookURL     string `yaml:"webhook_url"`
}

type EmailConfig struct {
	Enabled         bool                `yaml:"enabled"`
	SMTPHost        string              `yaml:"smtp_host"`
	SMTPPort        int                 `yaml:"smtp_port"`
	Username        string              `yaml:"username"`
	Password        string              `yaml:"password"`
	Sender          string              `yaml:"sender"`
	AlertReceiver   string              `yaml:"alert_receiver"`
	AlertReceivers  []string            `yaml:"alert_receivers"`
	SubjectTemplate string              `yaml:"subject_template"`
	BodyTemplate    string              `yaml:"body_template"`
	RecipientGroups map[string][]string `yaml:"recipient_groups"`
	UseTLS          bool                `yaml:"use_tls"`
	StartTLS        bool                `yaml:"start_tls"`
}

type ObservabilityConfig struct {
	RequestIDHeader string        `yaml:"request_id_header"`
	MetricsEnabled  bool          `yaml:"metrics_enabled"`
	Tracing         TracingConfig `yaml:"tracing"`
}

type TracingConfig struct {
	Enabled      bool    `yaml:"enabled"`
	ServiceName  string  `yaml:"service_name"`
	Environment  string  `yaml:"environment"`
	OTLPEndpoint string  `yaml:"otlp_endpoint"`
	SampleRatio  float64 `yaml:"sample_ratio"`
}

type JWTConfig struct {
	Secret               string `yaml:"secret"`
	AccessTokenExpire    int    `yaml:"access_token_expire"`
	RefreshTokenExpire   int    `yaml:"refresh_token_expire"`
	RefreshTokenRotation bool   `yaml:"refresh_token_rotation"`
	Issuer               string `yaml:"issuer"`
}

type CORSConfig struct {
	AllowOrigins     []string `yaml:"allow_origins"`
	AllowMethods     []string `yaml:"allow_methods"`
	AllowHeaders     []string `yaml:"allow_headers"`
	ExposeHeaders    []string `yaml:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

type LoggerConfig struct {
	Level      string `yaml:"level"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

type OAuthConfig struct {
	Github OAuthProviderConfig `yaml:"github"`
	Wechat OAuthProviderConfig `yaml:"wechat"`
}

type OAuthProviderConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURI  string `yaml:"redirect_uri"`
}

func (c OAuthProviderConfig) Ready() bool {
	return c.Enabled &&
		oauthConfigValueReady(c.ClientID) &&
		oauthConfigValueReady(c.ClientSecret) &&
		oauthConfigValueReady(c.RedirectURI)
}

type UploadConfig struct {
	StorageType   string              `yaml:"storage_type"`
	LocalPath     string              `yaml:"local_path"`
	PublicBaseURL string              `yaml:"public_base_url"`
	Local         LocalStorageConfig  `yaml:"local"`
	S3            ObjectStorageConfig `yaml:"s3"`
	MinIO         ObjectStorageConfig `yaml:"minio"`
	MaxSize       int                 `yaml:"max_size"` // MB
	AllowedTypes  []string            `yaml:"allowed_types"`
	Image         ImageConfig         `yaml:"image"`
}

type LocalStorageConfig struct {
	Path      string `yaml:"path"`
	URLPrefix string `yaml:"url_prefix"`
}

type ObjectStorageConfig struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	Region    string `yaml:"region"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	UseSSL    bool   `yaml:"use_ssl"`
}

type ImageConfig struct {
	MaxWidth        int `yaml:"max_width"`
	MaxHeight       int `yaml:"max_height"`
	ThumbnailWidth  int `yaml:"thumbnail_width"`
	ThumbnailHeight int `yaml:"thumbnail_height"`
}

func (c UploadConfig) EffectiveStorageType() string {
	storageType := strings.ToLower(strings.TrimSpace(c.StorageType))
	if storageType == "" {
		return "local"
	}
	return storageType
}

func (c UploadConfig) EffectiveLocalPath() string {
	if strings.TrimSpace(c.LocalPath) != "" {
		return c.LocalPath
	}
	if strings.TrimSpace(c.Local.Path) != "" {
		return c.Local.Path
	}
	return "./uploads"
}

func (c UploadConfig) EffectivePublicBaseURL() string {
	if strings.TrimSpace(c.PublicBaseURL) != "" {
		return c.PublicBaseURL
	}
	if strings.TrimSpace(c.Local.URLPrefix) != "" {
		return c.Local.URLPrefix
	}
	return "/uploads"
}

func (c UploadConfig) EffectiveLocalURLPrefix() string {
	candidate := strings.TrimSpace(c.Local.URLPrefix)
	if candidate == "" {
		candidate = c.EffectivePublicBaseURL()
	}
	if strings.HasPrefix(candidate, "/") {
		return candidate
	}
	parsed, err := url.Parse(candidate)
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}
	if candidate != "" {
		return "/" + strings.TrimLeft(candidate, "/")
	}
	return "/uploads"
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

// GetDSN returns the database connection string.
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
	// Phase 2B: schema-per-service — append search_path from DB_SEARCH_PATH env (skip if empty)
	if sp := os.Getenv("DB_SEARCH_PATH"); sp != "" {
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
