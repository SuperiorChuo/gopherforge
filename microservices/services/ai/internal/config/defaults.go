package config

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
			MaxOpenConns:           20,
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
				"X-Act-Tenant-ID",
				"Idempotency-Key",
				"Last-Event-ID",
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
			ImageModel: "dall-e-3",
		},
		Encryption: EncryptionConfig{
			// 仅开发环境；生产必须设置 ENCRYPTION_KEY（validate 强制 32 字节）。
			// 长度必须恰好 32：validate 对非空 key 一律校验长度，短一字节连
			// 开发环境都起不来（曾因 29 字节的默认值导致 ai-service 启动失败）。
			Key: "dev-only-key-32-bytes-padding!!!",
		},
		Hermes: HermesConfig{
			VerificationURI: "/hermes/mine",
		},
	}
}
