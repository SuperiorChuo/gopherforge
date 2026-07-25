package config

import (
	"strings"
	"testing"
)

// prodConfig 生产环境下的合规基线，各用例只改一项来断言该项的行为。
// 夹具用显式的 test- 前缀值，既不像真凭据也不落进弱值黑名单。
func prodConfig() Config {
	cfg := Defaults()
	cfg.App.Env = "production"
	cfg.JWT.Secret = "test-jwt-secret-for-unit-tests-0123456789"
	cfg.Database.Password = "test-db-password-for-unit-tests"
	cfg.Redis.Password = "test-redis-password-for-unit-tests"
	return cfg
}

func TestValidateAcceptsConfiguredProduction(t *testing.T) {
	if err := validate(prodConfig()); err != nil {
		t.Fatalf("validate() error = %v, want nil for a configured production config", err)
	}
}

// DB_PASSWORD 没有安全降级语义：空值、占位符、开发默认值一律拒绝启动。
func TestValidateRejectsWeakDBPasswordInProduction(t *testing.T) {
	for name, password := range map[string]string{
		"开发默认值": "123456",
		"空值":    "",
		"占位符":   "your-password",
		"待替换":   "change-me",
		"开发前缀":  "dev-db-password",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Database.Password = password
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
				t.Fatalf("validate() error = %v, want DB_PASSWORD rejection", err)
			}
		})
	}
}

// 开发环境保持零配置可跑：Defaults 里的弱密码不能被拦。
func TestValidateAllowsWeakDBPasswordOutsideProduction(t *testing.T) {
	cfg := Defaults()
	if !isWeakCredential(cfg.Database.Password) {
		t.Fatalf("Defaults().Database.Password = %q, want the weak local development default", cfg.Database.Password)
	}
	for _, env := range []string{"development", "staging", ""} {
		cfg.App.Env = env
		if err := validate(cfg); err != nil {
			t.Fatalf("validate() error = %v for APP_ENV=%q, want nil outside production", err, env)
		}
	}
}

// 两项都不合规时一次性报全，避免运维改一项重启一次。
func TestValidateReportsJWTSecretAndDBPasswordTogether(t *testing.T) {
	cfg := prodConfig()
	cfg.JWT.Secret = "local-dev-secret-change-me-32-chars"
	cfg.Database.Password = "123456"
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("validate() error = %v, want both JWT_SECRET and DB_PASSWORD reported", err)
	}
}

// Load 路径回归：DB_PASSWORD 由 applyEnv 注入，弱值必须在启动期就被拦下。
func TestLoadRejectsWeakDBPasswordFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "123456")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	if err := Load(); err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("Load() error = %v, want DB_PASSWORD rejection", err)
	}
}

func TestLoadAcceptsStrongDBPasswordFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v, want nil for a configured production environment", err)
	}
}

// REDIS_PASSWORD 同样没有安全降级语义：生产环境的 Redis 必须带认证，
// 空值意味着任何能连上 6379 的进程都能读写会话与验证码。
func TestValidateRejectsWeakRedisPasswordInProduction(t *testing.T) {
	for name, password := range map[string]string{
		"未设置":   "",
		"开发默认值": "123456",
		"服务名":   "redis",
		"占位符":   "your-password",
		"待替换":   "change-me",
		"开发前缀":  "dev-redis-password",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Redis.Password = password
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "REDIS_PASSWORD") {
				t.Fatalf("validate() error = %v, want REDIS_PASSWORD rejection", err)
			}
		})
	}
}

// 开发环境保持零配置可跑：Defaults 里的空 Redis 密码不能被拦。
func TestValidateAllowsWeakRedisPasswordOutsideProduction(t *testing.T) {
	cfg := Defaults()
	if !isWeakCredential(cfg.Redis.Password) {
		t.Fatalf("Defaults().Redis.Password = %q, want the weak local development default", cfg.Redis.Password)
	}
	for _, env := range []string{"development", "staging", ""} {
		cfg.App.Env = env
		if err := validate(cfg); err != nil {
			t.Fatalf("validate() error = %v for APP_ENV=%q, want nil outside production", err, env)
		}
	}
}

// 弱值判定是精确匹配且刻意不设长度下限：短口令只要不在弱值表里就放行。
// 真实部署里存在 9 字符的对象存储 access key，任何长度下限都会误杀。
func TestValidateImposesNoLengthFloorOnRedisPassword(t *testing.T) {
	cfg := prodConfig()
	cfg.Redis.Password = "test-pwd9"
	if len(cfg.Redis.Password) != 9 {
		t.Fatalf("fixture length = %d, want a 9-character value", len(cfg.Redis.Password))
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() error = %v, want nil for a short but non-weak password", err)
	}
}

// Load 路径回归：REDIS_PASSWORD 缺省为空，生产环境必须在启动期就被拦下。
func TestLoadRejectsMissingRedisPasswordFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "")
	if err := Load(); err == nil || !strings.Contains(err.Error(), "REDIS_PASSWORD") {
		t.Fatalf("Load() error = %v, want REDIS_PASSWORD rejection", err)
	}
}

// 对象存储凭据按 UPLOAD_STORAGE_TYPE 条件生效：本地磁盘存储根本没有凭据，
// 一刀切要求会让所有 local 部署起不来。
func TestValidateSkipsObjectStorageChecksForLocalStorage(t *testing.T) {
	for name, storageType := range map[string]string{
		"显式 local": "local",
		"缺省空值":     "",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Upload.StorageType = storageType
			cfg.Upload.S3 = ObjectStorageConfig{}
			cfg.Upload.MinIO = ObjectStorageConfig{}
			if err := validate(cfg); err != nil {
				t.Fatalf("validate() error = %v, want nil when local storage needs no object storage credentials", err)
			}
		})
	}
}

// 选了对象存储就必须给真凭据：弱值与缺失项一次性报全。
func TestValidateRejectsWeakObjectStorageCredentialsInProduction(t *testing.T) {
	t.Run("minio 默认凭据", func(t *testing.T) {
		cfg := prodConfig()
		cfg.Upload.StorageType = "minio"
		if cfg.Upload.MinIO.AccessKey != "minioadmin" || cfg.Upload.MinIO.SecretKey != "minioadmin" {
			t.Fatalf("Defaults().Upload.MinIO = %#v, want the weak local development credentials", cfg.Upload.MinIO)
		}
		err := validate(cfg)
		if err == nil {
			t.Fatal("validate() error = nil, want MinIO credential rejection")
		}
		for _, want := range []string{"UPLOAD_MINIO_ACCESS_KEY", "UPLOAD_MINIO_SECRET_KEY"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("validate() error = %v, want it to mention %s", err, want)
			}
		}
	})

	t.Run("minio 空凭据", func(t *testing.T) {
		cfg := prodConfig()
		cfg.Upload.StorageType = "minio"
		cfg.Upload.MinIO.AccessKey = ""
		cfg.Upload.MinIO.SecretKey = ""
		err := validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "UPLOAD_MINIO_ACCESS_KEY") {
			t.Fatalf("validate() error = %v, want UPLOAD_MINIO_ACCESS_KEY rejection", err)
		}
	})

	t.Run("s3 全空", func(t *testing.T) {
		cfg := prodConfig()
		cfg.Upload.StorageType = "s3"
		err := validate(cfg)
		if err == nil {
			t.Fatal("validate() error = nil, want an unconfigured S3 backend to be rejected")
		}
		// endpoint / bucket / region 是 internal/pkg/upload 自己就要求的必填项，
		// 这里只是把它从首次上传提前到启动期。
		for _, want := range []string{
			"UPLOAD_S3_ENDPOINT",
			"UPLOAD_S3_BUCKET",
			"UPLOAD_S3_REGION",
			"UPLOAD_S3_ACCESS_KEY",
			"UPLOAD_S3_SECRET_KEY",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("validate() error = %v, want it to mention %s", err, want)
			}
		}
	})

	// region 只对 s3 必填，MinIO 部署留空是合法的（与存储客户端口径一致）。
	t.Run("minio 不要求 region", func(t *testing.T) {
		cfg := prodConfig()
		cfg.Upload.StorageType = "minio"
		cfg.Upload.MinIO.Region = ""
		cfg.Upload.MinIO.AccessKey = "test-minio-access-key"
		cfg.Upload.MinIO.SecretKey = "test-minio-secret-key-0123456789"
		if err := validate(cfg); err != nil {
			t.Fatalf("validate() error = %v, want nil because MinIO does not need a region", err)
		}
	})
}

// 真实部署里的 MinIO access key 只有 9 字符，弱值判定不设长度下限，
// 不能仅因为短就拒绝启动。
func TestValidateAcceptsShortObjectStorageAccessKeyInProduction(t *testing.T) {
	cfg := prodConfig()
	cfg.Upload.StorageType = "minio"
	cfg.Upload.MinIO.AccessKey = "test-akid"
	cfg.Upload.MinIO.SecretKey = "test-minio-secret-key-0123456789"
	if len(cfg.Upload.MinIO.AccessKey) != 9 {
		t.Fatalf("fixture length = %d, want a 9-character access key", len(cfg.Upload.MinIO.AccessKey))
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() error = %v, want nil for a short but non-weak access key", err)
	}
}

// endpoint 形态沿用 internal/pkg/upload.objectStorageEndpoint 的规则。
func TestValidateRejectsMalformedObjectStorageEndpointInProduction(t *testing.T) {
	for name, endpoint := range map[string]string{
		"空值":     "",
		"带路径":    "http://storage.example.test/bucket",
		"不支持协议":  "ftp://storage.example.test",
		"裸主机带路径": "storage.example.test/bucket",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Upload.StorageType = "minio"
			cfg.Upload.MinIO.Endpoint = endpoint
			cfg.Upload.MinIO.AccessKey = "test-minio-access-key"
			cfg.Upload.MinIO.SecretKey = "test-minio-secret-key-0123456789"
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "UPLOAD_MINIO_ENDPOINT") {
				t.Fatalf("validate() error = %v, want UPLOAD_MINIO_ENDPOINT rejection", err)
			}
		})
	}
}

// Load 路径回归，按真实 MinIO 部署形态取值（长度与线上一致，内容为测试占位）：
// 强凭据放行，换回镜像默认凭据必须拒绝启动。
func TestLoadAcceptsObjectStorageDeploymentShapeFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	setDeploymentShapeEnv(t)
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v, want nil for a fully configured MinIO deployment", err)
	}
}

func TestLoadRejectsDefaultMinioCredentialsFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	setDeploymentShapeEnv(t)
	t.Setenv("UPLOAD_MINIO_ACCESS_KEY", "minioadmin")
	t.Setenv("UPLOAD_MINIO_SECRET_KEY", "minioadmin")
	err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want the image default MinIO credentials to be rejected")
	}
	for _, want := range []string{"UPLOAD_MINIO_ACCESS_KEY", "UPLOAD_MINIO_SECRET_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Load() error = %v, want it to mention %s", err, want)
		}
	}
}

func setDeploymentShapeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-pwd13")
	t.Setenv("REDIS_PASSWORD", "test-redis-pwd-018")
	t.Setenv("UPLOAD_STORAGE_TYPE", "minio")
	t.Setenv("UPLOAD_MINIO_ENDPOINT", "go-admin-kit-minio:9000")
	t.Setenv("UPLOAD_MINIO_BUCKET", "go-admin-kit")
	t.Setenv("UPLOAD_MINIO_REGION", "us-east-1")
	t.Setenv("UPLOAD_MINIO_ACCESS_KEY", "test-akid")
	t.Setenv("UPLOAD_MINIO_SECRET_KEY", "test-minio-secret-key-0123456789")
}
