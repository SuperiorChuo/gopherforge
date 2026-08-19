package config

import (
	"fmt"
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
	Notification  NotificationConfig
	NATS          NATSConfig
	Codegen       CodegenConfig
	EdgeCert      EdgeCertConfig
	InternalToken string
}

// EdgeCertConfig 是边缘证书生命周期的运行配置。加密密钥经
// envsecret 从 /run/secrets 读取；值是 base64(raw 32 bytes)，不允许明文进仓。
type EdgeCertConfig struct {
	CurrentKeyID        string
	CurrentKeyBase64    string
	PreviousKeyID       string
	PreviousKeyBase64   string
	StorageRoot         string
	TraefikDynamicDir   string
	GatewayTLSAddress   string
	WorkerEnabled       bool
	RenewBeforeDays     int
	TaskPollSeconds     int
	ChallengeTTLMinutes int
	ClearLegacySecrets  bool
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

// KeyMaterials decodes configured base64(raw 32-byte) edge certificate keys.
// Empty current configuration means Issue/Export are disabled, while external
// TLS probing and list operations can remain available.
func (cfg EdgeCertConfig) KeyMaterials() (currentID string, current []byte, previousID string, previous []byte, err error) {
	currentID = strings.TrimSpace(cfg.CurrentKeyID)
	previousID = strings.TrimSpace(cfg.PreviousKeyID)
	if currentID != "" {
		current, err = decodeEdgeCertKey("EDGE_CERT_ENCRYPTION_KEY", cfg.CurrentKeyBase64)
		if err != nil {
			return "", nil, "", nil, err
		}
	}
	if previousID != "" {
		previous, err = decodeEdgeCertKey("EDGE_CERT_PREVIOUS_ENCRYPTION_KEY", cfg.PreviousKeyBase64)
		if err != nil {
			return "", nil, "", nil, err
		}
	}
	return currentID, current, previousID, previous, nil
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
