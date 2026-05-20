package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
	Observability ObservabilityConfig `yaml:"observability"`
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
	Charset                string `yaml:"charset"`
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
	TrustedProxies []string           `yaml:"trusted_proxies"`
	Headers        SecurityHeaders    `yaml:"headers"`
	RateLimit      RateLimitConfig    `yaml:"rate_limit"`
	LoginLimit     LoginLimitConfig   `yaml:"login_limit"`
	DefaultAdmin   DefaultAdminConfig `yaml:"default_admin"`
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
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURI  string `yaml:"redirect_uri"`
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

var (
	Cfg Config
)

// LoadConfig loads the configuration file.
func LoadConfig(filePath string) error {
	// Read the configuration file.
	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML.
	if err := yaml.Unmarshal(file, &Cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Replace environment variables.
	replaceEnvVars(&Cfg)

	return nil
}

// Validate checks high-risk configuration combinations.
func Validate() error {
	if Cfg.CORS.AllowCredentials && containsString(Cfg.CORS.AllowOrigins, "*") {
		return fmt.Errorf("CORS cannot use '*' when credentials are enabled")
	}
	switch Cfg.Upload.EffectiveStorageType() {
	case "local", "s3", "minio":
	default:
		return fmt.Errorf("upload storage_type must be one of: local, s3, minio")
	}
	if isProductionEnv(Cfg.App.Env) {
		if err := validateProductionSafety(Cfg); err != nil {
			return err
		}
	}
	if Cfg.Observability.Tracing.SampleRatio < 0 || Cfg.Observability.Tracing.SampleRatio > 1 {
		return fmt.Errorf("observability.tracing.sample_ratio must be between 0 and 1")
	}
	return nil
}

func validateProductionSafety(config Config) error {
	issues := make([]string, 0)

	if !isStrongSecret(config.JWT.Secret, 32) {
		issues = append(issues, "jwt.secret must be at least 32 characters and must not use a default or placeholder value")
	}
	if isWeakCredential(config.Database.Password) {
		issues = append(issues, "database.password must not be empty, default, weak, or placeholder")
	}
	if isWeakCredential(config.Redis.Password) {
		issues = append(issues, "redis.password must not be empty, default, weak, or placeholder")
	}
	switch config.Upload.EffectiveStorageType() {
	case "s3":
		issues = appendObjectStorageIssues(issues, "upload.s3", config.Upload.S3, true)
	case "minio":
		issues = appendObjectStorageIssues(issues, "upload.minio", config.Upload.MinIO, false)
	}

	if len(issues) > 0 {
		return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
	}
	return nil
}

func appendObjectStorageIssues(issues []string, path string, storage ObjectStorageConfig, requireRegion bool) []string {
	issues = appendObjectStorageEndpointIssues(issues, path, storage.Endpoint)
	if strings.TrimSpace(storage.Bucket) == "" {
		issues = append(issues, path+".bucket must be set")
	}
	if requireRegion && strings.TrimSpace(storage.Region) == "" {
		issues = append(issues, path+".region must be set")
	}
	if isWeakCredential(storage.AccessKey) {
		issues = append(issues, path+".access_key must not be empty, default, weak, or placeholder")
	}
	if isWeakCredential(storage.SecretKey) {
		issues = append(issues, path+".secret_key must not be empty, default, weak, or placeholder")
	}
	return issues
}

func appendObjectStorageEndpointIssues(issues []string, path string, endpoint string) []string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return append(issues, path+".endpoint must be set")
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			return append(issues, path+".endpoint must be a valid host or URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return append(issues, path+".endpoint must use http or https")
		}
		if strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return append(issues, path+".endpoint must not include path, query, or fragment")
		}
		return issues
	}
	if strings.ContainsAny(endpoint, "/\\?#") {
		return append(issues, path+".endpoint must not include path, query, or fragment")
	}
	return issues
}

func isProductionEnv(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production")
}

func isStrongSecret(value string, minLength int) bool {
	value = strings.TrimSpace(value)
	return len(value) >= minLength && !isPlaceholderValue(value)
}

func isWeakCredential(value string) bool {
	normalized := normalizeSecretValue(value)
	if normalized == "" || isPlaceholderValue(normalized) {
		return true
	}
	weakValues := map[string]struct{}{
		"123456":                {},
		"access-key":            {},
		"accesskey":             {},
		"admin":                 {},
		"aws-access-key-id":     {},
		"aws-secret-access-key": {},
		"aws_access_key_id":     {},
		"aws_secret_access_key": {},
		"awsaccesskeyid":        {},
		"awssecretaccesskey":    {},
		"changeme":              {},
		"default":               {},
		"demo":                  {},
		"development":           {},
		"example":               {},
		"local":                 {},
		"minioadmin":            {},
		"password":              {},
		"redis":                 {},
		"root":                  {},
		"sample":                {},
		"go-admin-kit":          {},
		"secret-key":            {},
		"secretkey":             {},
		"test":                  {},
		"test123":               {},
	}
	_, ok := weakValues[normalized]
	return ok
}

func isPlaceholderValue(value string) bool {
	normalized := normalizeSecretValue(value)
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

func normalizeSecretValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// replaceEnvVars replaces environment variables in the configuration.
func replaceEnvVars(config *Config) {
	config.App.Env = getEnvString("APP_ENV", config.App.Env)
	config.App.Port = getEnvInt("APP_PORT", config.App.Port)

	config.Database.Host = getEnvString("DB_HOST", config.Database.Host)
	config.Database.Port = getEnvInt("DB_PORT", config.Database.Port)
	config.Database.User = getEnvString("DB_USER", config.Database.User)
	config.Database.Password = getEnvString("DB_PASSWORD", config.Database.Password)
	config.Database.DBName = getEnvString("DB_NAME", config.Database.DBName)
	config.Database.ConnMaxLifetimeSeconds = getEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", config.Database.ConnMaxLifetimeSeconds)
	config.Database.ConnMaxIdleTimeSeconds = getEnvInt("DB_CONN_MAX_IDLE_TIME_SECONDS", config.Database.ConnMaxIdleTimeSeconds)

	config.Redis.Host = getEnvString("REDIS_HOST", config.Redis.Host)
	config.Redis.Port = getEnvInt("REDIS_PORT", config.Redis.Port)
	config.Redis.Password = getEnvString("REDIS_PASSWORD", config.Redis.Password)
	config.Redis.DB = getEnvInt("REDIS_DB", config.Redis.DB)

	config.JWT.Secret = getEnvString("JWT_SECRET", config.JWT.Secret)
	config.JWT.RefreshTokenRotation = getEnvBool("JWT_REFRESH_TOKEN_ROTATION", config.JWT.RefreshTokenRotation)
	config.Upload.StorageType = getEnvString("UPLOAD_STORAGE_TYPE", config.Upload.StorageType)
	config.Upload.LocalPath = getEnvString("UPLOAD_LOCAL_PATH", config.Upload.LocalPath)
	config.Upload.PublicBaseURL = getEnvString("UPLOAD_PUBLIC_BASE_URL", config.Upload.PublicBaseURL)
	config.Upload.Local.Path = getEnvString("UPLOAD_LOCAL_PATH", config.Upload.Local.Path)
	config.Upload.Local.URLPrefix = getEnvString("UPLOAD_LOCAL_URL_PREFIX", config.Upload.Local.URLPrefix)
	config.Upload.S3.Endpoint = getEnvString("UPLOAD_S3_ENDPOINT", config.Upload.S3.Endpoint)
	config.Upload.S3.Bucket = getEnvString("UPLOAD_S3_BUCKET", config.Upload.S3.Bucket)
	config.Upload.S3.Region = getEnvString("UPLOAD_S3_REGION", config.Upload.S3.Region)
	config.Upload.S3.AccessKey = getEnvString("UPLOAD_S3_ACCESS_KEY", config.Upload.S3.AccessKey)
	config.Upload.S3.SecretKey = getEnvString("UPLOAD_S3_SECRET_KEY", config.Upload.S3.SecretKey)
	config.Upload.S3.UseSSL = getEnvBool("UPLOAD_S3_USE_SSL", config.Upload.S3.UseSSL)
	config.Upload.MinIO.Endpoint = getEnvString("UPLOAD_MINIO_ENDPOINT", config.Upload.MinIO.Endpoint)
	config.Upload.MinIO.Bucket = getEnvString("UPLOAD_MINIO_BUCKET", config.Upload.MinIO.Bucket)
	config.Upload.MinIO.Region = getEnvString("UPLOAD_MINIO_REGION", config.Upload.MinIO.Region)
	config.Upload.MinIO.AccessKey = getEnvString("UPLOAD_MINIO_ACCESS_KEY", config.Upload.MinIO.AccessKey)
	config.Upload.MinIO.SecretKey = getEnvString("UPLOAD_MINIO_SECRET_KEY", config.Upload.MinIO.SecretKey)
	config.Upload.MinIO.UseSSL = getEnvBool("UPLOAD_MINIO_USE_SSL", config.Upload.MinIO.UseSSL)
	config.CORS.AllowOrigins = getEnvStringSlice("CORS_ALLOW_ORIGINS", config.CORS.AllowOrigins)
	config.CORS.AllowCredentials = getEnvBool("CORS_ALLOW_CREDENTIALS", config.CORS.AllowCredentials)
	config.Security.TrustedProxies = getEnvStringSlice("TRUSTED_PROXIES", config.Security.TrustedProxies)
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
	config.Observability.MetricsEnabled = getEnvBool("METRICS_ENABLED", config.Observability.MetricsEnabled)
	config.Observability.Tracing.Enabled = getEnvBool("TRACING_ENABLED", config.Observability.Tracing.Enabled)
	config.Observability.Tracing.ServiceName = getEnvString("OTEL_SERVICE_NAME", config.Observability.Tracing.ServiceName)
	config.Observability.Tracing.ServiceName = getEnvString("TRACING_SERVICE_NAME", config.Observability.Tracing.ServiceName)
	config.Observability.Tracing.Environment = getEnvString("TRACING_ENVIRONMENT", config.Observability.Tracing.Environment)
	config.Observability.Tracing.OTLPEndpoint = getEnvString("OTEL_EXPORTER_OTLP_ENDPOINT", config.Observability.Tracing.OTLPEndpoint)
	config.Observability.Tracing.OTLPEndpoint = getEnvString("TRACING_OTLP_ENDPOINT", config.Observability.Tracing.OTLPEndpoint)
	config.Observability.Tracing.SampleRatio = getEnvFloat64("TRACING_SAMPLE_RATIO", config.Observability.Tracing.SampleRatio)
	config.OAuth.Github.ClientID = getEnvString("GITHUB_CLIENT_ID", config.OAuth.Github.ClientID)
	config.OAuth.Github.ClientSecret = getEnvString("GITHUB_CLIENT_SECRET", config.OAuth.Github.ClientSecret)
	config.OAuth.Github.RedirectURI = getEnvString("GITHUB_REDIRECT_URI", config.OAuth.Github.RedirectURI)
	config.OAuth.Wechat.ClientID = getEnvString("WECHAT_CLIENT_ID", config.OAuth.Wechat.ClientID)
	config.OAuth.Wechat.ClientSecret = getEnvString("WECHAT_CLIENT_SECRET", config.OAuth.Wechat.ClientSecret)
	config.OAuth.Wechat.RedirectURI = getEnvString("WECHAT_REDIRECT_URI", config.OAuth.Wechat.RedirectURI)
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

// GetDSN returns the database connection string.
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.Charset)
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
