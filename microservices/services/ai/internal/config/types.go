package config

import (
	"fmt"
	"os"
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
	AI            AIConfig
	Encryption    EncryptionConfig
	Hermes        HermesConfig
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

// AIConfig holds model-provider settings for the AI service. An empty APIKey
// means the service is not configured: it still starts and serves health
// checks, but every AI endpoint returns 503 AI_NOT_CONFIGURED.
type AIConfig struct {
	// Provider selects the primary chat provider: "openai" or "anthropic".
	Provider string
	// BaseURL overrides the provider endpoint (OpenAI-compatible vendors such
	// as DeepSeek, Qwen, or Ollama are reached through this).
	BaseURL string
	// APIKey authenticates against the primary provider; empty = unconfigured.
	APIKey string
	// ChatModel is the model name used for chat completions.
	ChatModel string
	// EmbedModel is the model name used for embeddings.
	EmbedModel string
	// ImageModel is the model name used for image generation (OpenAI-compatible
	// /v1/images/generations); empty falls back to the provider default.
	ImageModel string
	// EmbedProvider optionally routes embeddings to a different provider;
	// empty means embeddings follow the primary provider.
	EmbedProvider string
	// EmbedBaseURL overrides the embedding provider endpoint.
	EmbedBaseURL string
	// EmbedAPIKey authenticates against the embedding provider; empty means
	// the primary APIKey is reused.
	EmbedAPIKey string
}

// EncryptionConfig 加密配置（用于敏感数据如API Key的加密存储）
type EncryptionConfig struct {
	// Key 32字节密钥（AES-256）
	Key string
}

// HermesConfig is intentionally separate from EncryptionConfig. Policy
// signing is an Ed25519 trust root shared with enrolled managers, not a use of
// the AES key that protects provider credentials at rest.
type HermesConfig struct {
	PolicySigningKey string
	// SkillSigningPublicKey 是技能供应链验签信任根（base64 Ed25519 公钥）。
	// 配置后手动上传技能必须带有效签名；未配置则降级不验签（同 policy signer）。
	SkillSigningPublicKey string
	VerificationURI       string
}

// Configured reports whether the primary provider has credentials.
func (c AIConfig) Configured() bool {
	return strings.TrimSpace(c.APIKey) != ""
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
	// Phase 2B：schema-per-service——search_path 读 DB_SEARCH_PATH env（空则不追加）
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
