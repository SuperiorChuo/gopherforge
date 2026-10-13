package config

import (
	"strings"
	"testing"
)

// prodConfig 生产环境下的合规基线，各用例只改一项来断言该项的行为。
func prodConfig() Config {
	return Config{
		AppEnv:        "production",
		JWTSecret:     "test-jwt-secret-for-unit-tests-0123456789",
		DBPassword:    "test-db-password-for-unit-tests",
		RedisPassword: "test-redis-password-for-unit-tests",
	}
}

func TestValidateAllowsDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	if err := validate(build()); err != nil {
		t.Fatalf("validate() error = %v, want nil for development defaults", err)
	}
}

// JWT_SECRET / DB_PASSWORD 没有安全降级语义，占位符必须拒绝启动。
func TestValidateRejectsPlaceholderJWTSecret(t *testing.T) {
	c := prodConfig()
	c.JWTSecret = "local-dev-secret-change-me-32-chars"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("validate() error = %v, want JWT_SECRET placeholder error", err)
	}
}

func TestValidateRejectsShortJWTSecret(t *testing.T) {
	c := prodConfig()
	c.JWTSecret = "8c3f7a1e5b2d9046"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("validate() error = %v, want JWT_SECRET length error", err)
	}
}

func TestValidateRejectsDefaultDBPassword(t *testing.T) {
	c := prodConfig()
	c.DBPassword = "123456"
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("validate() error = %v, want DB_PASSWORD weak error", err)
	}
}

func TestValidateAcceptsConfiguredProduction(t *testing.T) {
	if err := validate(prodConfig()); err != nil {
		t.Fatalf("validate() error = %v, want nil for a configured production config", err)
	}
}

// access_token 中控落在 Redis，裸奔 Redis = 公众号 token 裸奔。
func TestValidateRejectsEmptyRedisPassword(t *testing.T) {
	c := prodConfig()
	c.RedisPassword = ""
	err := validate(c)
	if err == nil || !strings.Contains(err.Error(), "REDIS_PASSWORD") {
		t.Fatalf("validate() error = %v, want REDIS_PASSWORD error", err)
	}
}

// compose 注入的开发占位符必须在黑名单里。
func TestPlaceholderBlacklistCoversDeployedDevTokens(t *testing.T) {
	for _, v := range []string{"dev-notify-internal-token", "dev-im-ai-internal-token"} {
		if !isPlaceholderValue(v) {
			t.Fatalf("isPlaceholderValue(%q) = false, want true", v)
		}
	}
}

func TestIsProductionEnvIsCaseInsensitive(t *testing.T) {
	for _, env := range []string{"production", "Production", " PRODUCTION "} {
		if !isProductionEnv(env) {
			t.Fatalf("isProductionEnv(%q) = false, want true", env)
		}
	}
	for _, env := range []string{"development", "staging", ""} {
		if isProductionEnv(env) {
			t.Fatalf("isProductionEnv(%q) = true, want false", env)
		}
	}
}
