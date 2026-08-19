package config

import (
	envsecret "github.com/go-admin-kit/services/shared/pkg/envsecret"
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

	config.Upload.StorageType = getEnvString("UPLOAD_STORAGE_TYPE", config.Upload.StorageType)
	config.Upload.LocalPath = getEnvString("UPLOAD_LOCAL_PATH", config.Upload.LocalPath)
	config.Upload.PublicBaseURL = getEnvString("UPLOAD_PUBLIC_BASE_URL", config.Upload.PublicBaseURL)
	config.Upload.Local.Path = getEnvString("UPLOAD_LOCAL_PATH", config.Upload.Local.Path)
	config.Upload.Local.URLPrefix = getEnvString("UPLOAD_LOCAL_URL_PREFIX", config.Upload.Local.URLPrefix)
	config.Upload.S3.Endpoint = getEnvString("UPLOAD_S3_ENDPOINT", config.Upload.S3.Endpoint)
	config.Upload.S3.Bucket = getEnvString("UPLOAD_S3_BUCKET", config.Upload.S3.Bucket)
	config.Upload.S3.Region = getEnvString("UPLOAD_S3_REGION", config.Upload.S3.Region)
	config.Upload.S3.AccessKey = getSecretString("UPLOAD_S3_ACCESS_KEY", config.Upload.S3.AccessKey)
	config.Upload.S3.SecretKey = getSecretString("UPLOAD_S3_SECRET_KEY", config.Upload.S3.SecretKey)
	config.Upload.S3.UseSSL = getEnvBool("UPLOAD_S3_USE_SSL", config.Upload.S3.UseSSL)
	config.Upload.S3.BucketLookup = getEnvString("UPLOAD_S3_BUCKET_LOOKUP", config.Upload.S3.BucketLookup)
	config.Upload.MinIO.Endpoint = getEnvString("UPLOAD_MINIO_ENDPOINT", config.Upload.MinIO.Endpoint)
	config.Upload.MinIO.Bucket = getEnvString("UPLOAD_MINIO_BUCKET", config.Upload.MinIO.Bucket)
	config.Upload.MinIO.Region = getEnvString("UPLOAD_MINIO_REGION", config.Upload.MinIO.Region)
	config.Upload.MinIO.AccessKey = getSecretString("UPLOAD_MINIO_ACCESS_KEY", config.Upload.MinIO.AccessKey)
	config.Upload.MinIO.SecretKey = getSecretString("UPLOAD_MINIO_SECRET_KEY", config.Upload.MinIO.SecretKey)
	config.Upload.MinIO.UseSSL = getEnvBool("UPLOAD_MINIO_USE_SSL", config.Upload.MinIO.UseSSL)
	config.Upload.MinIO.BucketLookup = getEnvString("UPLOAD_MINIO_BUCKET_LOOKUP", config.Upload.MinIO.BucketLookup)
	config.Upload.URLSignSecret = getEnvString("UPLOAD_URL_SIGN_SECRET", config.Upload.URLSignSecret)
	config.Upload.MaxSize = getEnvInt("UPLOAD_MAX_SIZE", config.Upload.MaxSize)
	config.Upload.URLSignTTLSeconds = getEnvInt("UPLOAD_URL_SIGN_TTL_SECONDS", config.Upload.URLSignTTLSeconds)
}

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
