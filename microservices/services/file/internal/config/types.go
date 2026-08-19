package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
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
	NATS          NATSConfig
	Upload        UploadConfig
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
	// URL is the NATS server URL; empty disables event publishing.
	URL string
}

// UploadConfig mirrors the monolith's upload configuration so pkg/upload code
// copied from the monolith keeps working unchanged.
type UploadConfig struct {
	StorageType   string
	LocalPath     string
	PublicBaseURL string
	Local         LocalStorageConfig
	S3            ObjectStorageConfig
	MinIO         ObjectStorageConfig
	MaxSize       int // MB
	AllowedTypes  []string
	Image         ImageConfig
	// URLSignSecret signs /uploads URLs (HMAC). Empty falls back to JWT.Secret.
	URLSignSecret string
	// URLSignTTLSeconds bounds how long a signed /uploads URL stays valid.
	URLSignTTLSeconds int
}

type LocalStorageConfig struct {
	Path      string
	URLPrefix string
}

type ObjectStorageConfig struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	// BucketLookup 寻址风格：auto（缺省）| dns（virtual-host，阿里 OSS/腾讯
	// COS 官方端点）| path（MinIO/IP 端点）。S3 兼容云的主要差异点。
	BucketLookup string
}

type ImageConfig struct {
	MaxWidth        int
	MaxHeight       int
	ThumbnailWidth  int
	ThumbnailHeight int
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
