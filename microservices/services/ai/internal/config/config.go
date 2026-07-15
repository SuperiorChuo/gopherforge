// Package config provides 12-factor, environment-only configuration for the
// AI service. It intentionally keeps the same struct shape, global Cfg
// variable, helper methods, and environment variable names as the monolith's
// config package so that code copied from the monolith keeps working
// unchanged and docker-compose environments stay uniform.
package config

import (
	"fmt"
	"os"
	"strconv"
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
	// EmbedProvider optionally routes embeddings to a different provider;
	// empty means embeddings follow the primary provider.
	EmbedProvider string
	// EmbedBaseURL overrides the embedding provider endpoint.
	EmbedBaseURL string
	// EmbedAPIKey authenticates against the embedding provider; empty means
	// the primary APIKey is reused.
	EmbedAPIKey string
}

// Configured reports whether the primary provider has credentials.
func (c AIConfig) Configured() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

var Cfg Config

// Defaults returns the local-development configuration. Values match the
// monolith's configs/config.yaml so both services behave identically against
// the shared Postgres/Redis, except App.Port which defaults to 8087.
func Defaults() Config {
	return Config{
		App: AppCfg{
			Name:    "go-admin-kit-ai",
			Version: "1.0.0",
			Env:     "development",
			Port:    8087,
		},
		Database: DatabaseConfig{
			Driver:                 "postgres",
			Host:                   "localhost",
			Port:                   5432,
			User:                   "postgres",
			Password:               "123456",
			DBName:                 "go_admin_kit",
			SSLMode:                "disable",
			MaxIdleConns:           10,
			MaxOpenConns:           100,
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
			TrustedProxies:       []string{"127.0.0.1"},
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
				ServiceName:  "go-admin-kit-ai",
				Environment:  "development",
				OTLPEndpoint: "localhost:4317",
				SampleRatio:  1.0,
			},
		},
		NATS: NATSConfig{URL: ""},
		AI: AIConfig{
			Provider:   "openai",
			ChatModel:  "gpt-4o-mini",
			EmbedModel: "text-embedding-3-small",
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
	config.Database.Password = getEnvString("DB_PASSWORD", config.Database.Password)
	config.Database.DBName = getEnvString("DB_NAME", config.Database.DBName)
	config.Database.SSLMode = getEnvString("DB_SSLMODE", config.Database.SSLMode)
	config.Database.ConnMaxLifetimeSeconds = getEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", config.Database.ConnMaxLifetimeSeconds)
	config.Database.ConnMaxIdleTimeSeconds = getEnvInt("DB_CONN_MAX_IDLE_TIME_SECONDS", config.Database.ConnMaxIdleTimeSeconds)

	config.Redis.Host = getEnvString("REDIS_HOST", config.Redis.Host)
	config.Redis.Port = getEnvInt("REDIS_PORT", config.Redis.Port)
	config.Redis.Password = getEnvString("REDIS_PASSWORD", config.Redis.Password)
	config.Redis.DB = getEnvInt("REDIS_DB", config.Redis.DB)

	config.JWT.Secret = getEnvString("JWT_SECRET", config.JWT.Secret)
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
	config.OAuth.Github.ClientSecret = getEnvString("GITHUB_CLIENT_SECRET", config.OAuth.Github.ClientSecret)
	config.OAuth.Github.RedirectURI = getEnvString("GITHUB_REDIRECT_URI", config.OAuth.Github.RedirectURI)
	config.OAuth.Wechat.Enabled = getEnvBool("WECHAT_OAUTH_ENABLED", config.OAuth.Wechat.Enabled)
	config.OAuth.Wechat.ClientID = getEnvString("WECHAT_CLIENT_ID", config.OAuth.Wechat.ClientID)
	config.OAuth.Wechat.ClientSecret = getEnvString("WECHAT_CLIENT_SECRET", config.OAuth.Wechat.ClientSecret)
	config.OAuth.Wechat.RedirectURI = getEnvString("WECHAT_REDIRECT_URI", config.OAuth.Wechat.RedirectURI)

	config.NATS.URL = getEnvString("NATS_URL", config.NATS.URL)

	config.AI.Provider = strings.ToLower(strings.TrimSpace(getEnvString("AI_PROVIDER", config.AI.Provider)))
	config.AI.BaseURL = getEnvString("AI_BASE_URL", config.AI.BaseURL)
	config.AI.APIKey = getEnvString("AI_API_KEY", config.AI.APIKey)
	config.AI.ChatModel = getEnvString("AI_CHAT_MODEL", config.AI.ChatModel)
	config.AI.EmbedModel = getEnvString("AI_EMBED_MODEL", config.AI.EmbedModel)
	config.AI.EmbedProvider = strings.ToLower(strings.TrimSpace(getEnvString("AI_EMBED_PROVIDER", config.AI.EmbedProvider)))
	config.AI.EmbedBaseURL = getEnvString("AI_EMBED_BASE_URL", config.AI.EmbedBaseURL)
	config.AI.EmbedAPIKey = getEnvString("AI_EMBED_API_KEY", config.AI.EmbedAPIKey)
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
	if isProductionEnv(cfg.App.Env) {
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			return fmt.Errorf("JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
	}
	if cfg.AI.Provider != "" && cfg.AI.Provider != "openai" && cfg.AI.Provider != "anthropic" {
		return fmt.Errorf("AI_PROVIDER must be one of: openai, anthropic")
	}
	if cfg.AI.EmbedProvider != "" && cfg.AI.EmbedProvider != "openai" && cfg.AI.EmbedProvider != "anthropic" {
		return fmt.Errorf("AI_EMBED_PROVIDER must be one of: openai, anthropic")
	}
	return nil
}

func isProductionEnv(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production")
}

func isStrongSecret(value string, minLength int) bool {
	value = strings.TrimSpace(value)
	return len(value) >= minLength && !isPlaceholderValue(value)
}

func isPlaceholderValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	placeholderValues := map[string]struct{}{
		"change-me":                           {},
		"changeme":                            {},
		"local-dev-secret-change-me-32-chars": {},
		"replace-me":                          {},
		"replace-with-at-least-32-random-characters": {},
		"your-password":   {},
		"your-secret-key": {},
	}
	if _, ok := placeholderValues[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "placeholder") ||
		strings.Contains(normalized, "replace-with") ||
		strings.HasPrefix(normalized, "your-")
}

func oauthConfigValueReady(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !isPlaceholderValue(value)
}

func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
