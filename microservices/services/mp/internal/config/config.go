// Package config provides environment-only configuration for mp-service
// (微信公众号), following the im/cc/crm convention. 真实密钥（AppSecret /
// EncodingAESKey）不进代码：mp_accounts 只存引用名，明文走 .env 或 system_settings。
package config

import (
	"github.com/go-admin-kit/services/shared/pkg/envsecret"
	"fmt"
	"log"
	"os"
	"strings"

	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

type Config struct {
	AppPort    string
	AppEnv     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	JWTSecret  string

	// Redis 供 access_token 中控（M2）。空则 M1 仍可跑（不刷 token）。
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       string

	// AIBaseURL 为 ai-service 内网地址（M5 生成文章/封面）。空则禁用 AI 生成。
	AIBaseURL string
}

func Load() Config {
	c := build()
	if err := validate(c); err != nil {
		log.Fatalf("config: %v", err)
	}
	return c
}

func build() Config {
	return Config{
		AppPort:    getenv("APP_PORT", "8092"),
		AppEnv:     getenv("APP_ENV", "development"),
		DBHost:     getenv("DB_HOST", "127.0.0.1"),
		DBPort:     getenv("DB_PORT", "5432"),
		DBUser:     getenv("DB_USER", "postgres"),
		DBPassword: getSecret("DB_PASSWORD", "123456"),
		DBName:     getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:  getenv("DB_SSLMODE", "disable"),
		JWTSecret:  getSecret("JWT_SECRET", "local-dev-secret-change-me-32-chars"),

		RedisHost:     getenv("REDIS_HOST", "127.0.0.1"),
		RedisPort:     getenv("REDIS_PORT", "6379"),
		RedisPassword: getSecret("REDIS_PASSWORD", ""),
		RedisDB:       getenv("REDIS_DB", "0"),

		AIBaseURL: getenv("AI_BASE_URL", "http://go-admin-kit-ai:8087"),
	}
}

// validate APP_ENV=production 时拒绝带开发默认值/占位符启动，口径与 auth、
// monitor 等核心服务一致（占位符黑名单 + 弱值黑名单 + 32 位密钥门槛）。
// 轻量服务不引 shared 依赖（Docker 构建上下文只含本服务目录），故为本地副本。
//
// AppSecret / EncodingAESKey 不在这里校验：mp_accounts 存的是引用名，明文由
// wxclient 运行时按名解析（见 EnvSecretResolver），启动期拿不到也不该拿。
func validate(c Config) error {
	if !isProductionEnv(c.AppEnv) {
		return nil
	}
	issues := make([]string, 0)

	if !isStrongSecret(c.JWTSecret, 32) {
		issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
	}
	if isWeakCredential(c.DBPassword) {
		issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
	}
	// access_token 中控存在 Redis 里，裸奔的 Redis 等于公众号 token 裸奔。
	if isWeakCredential(c.RedisPassword) {
		issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
	}

	if len(issues) > 0 {
		return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
	}
	return nil
}

// ProductionMode 供使用点判断是否要 fail closed。
func (c Config) ProductionMode() bool { return isProductionEnv(c.AppEnv) } // 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

func (c Config) DSN() string {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
	// Phase 2B：schema-per-service——search_path 读 DB_SEARCH_PATH env（空则不追加）
	if sp := os.Getenv("DB_SEARCH_PATH"); sp != "" {
		dsn += " search_path=" + sp
	}
	return dsn
}

func getSecret(key, fallback string) string {
	return envsecret.Get(key, fallback)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
