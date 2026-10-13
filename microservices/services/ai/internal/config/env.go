package config

import (
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"os"
	"strconv"
	"strings"
)

func applyEnv(config *Config) {
	config.App.Env = getEnvString("APP_ENV", config.App.Env)
	config.App.Port = getEnvInt("APP_PORT", config.App.Port)

	config.Database.Host = getEnvString("DB_HOST", config.Database.Host)
	config.Database.Port = getEnvInt("DB_PORT", config.Database.Port)
	config.Database.User = getEnvString("DB_USER", config.Database.User)
	// Secrets prefer /run/secrets then env (shared/pkg/envsecret).
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

	config.AI.Provider = strings.ToLower(strings.TrimSpace(getEnvString("AI_PROVIDER", config.AI.Provider)))
	config.AI.BaseURL = getEnvString("AI_BASE_URL", config.AI.BaseURL)
	config.AI.APIKey = getSecretString("AI_API_KEY", config.AI.APIKey)
	config.AI.ChatModel = getEnvString("AI_CHAT_MODEL", config.AI.ChatModel)
	config.AI.EmbedModel = getEnvString("AI_EMBED_MODEL", config.AI.EmbedModel)
	config.AI.ImageModel = getEnvString("AI_IMAGE_MODEL", config.AI.ImageModel)
	config.AI.EmbedProvider = strings.ToLower(strings.TrimSpace(getEnvString("AI_EMBED_PROVIDER", config.AI.EmbedProvider)))
	config.AI.EmbedBaseURL = getEnvString("AI_EMBED_BASE_URL", config.AI.EmbedBaseURL)
	config.AI.EmbedAPIKey = getSecretString("AI_EMBED_API_KEY", config.AI.EmbedAPIKey)

	config.Encryption.Key = getSecretString("ENCRYPTION_KEY", config.Encryption.Key)
	config.Hermes.PolicySigningKey = getSecretString("HERMES_POLICY_SIGNING_KEY", config.Hermes.PolicySigningKey)
	config.Hermes.SkillSigningPublicKey = getSecretString("HERMES_SKILL_SIGNING_PUBLIC_KEY", config.Hermes.SkillSigningPublicKey)
	config.Hermes.VerificationURI = getEnvString("HERMES_VERIFICATION_URI", config.Hermes.VerificationURI)
}

// getSecretString reads sensitive config: Swarm secrets first, then env.
func getSecretString(key, fallback string) string {
	return envsecret.Get(key, fallback)
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
