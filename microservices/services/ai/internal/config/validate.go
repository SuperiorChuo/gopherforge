package config

import (
	"fmt"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
)

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
		if len(cfg.Encryption.Key) != 32 {
			return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes in production")
		}
	}
	if cfg.Encryption.Key != "" && len(cfg.Encryption.Key) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (got %d)", len(cfg.Encryption.Key))
	}
	if cfg.AI.Provider != "" && cfg.AI.Provider != "openai" && cfg.AI.Provider != "anthropic" {
		return fmt.Errorf("AI_PROVIDER must be one of: openai, anthropic")
	}
	if cfg.AI.EmbedProvider != "" && cfg.AI.EmbedProvider != "openai" && cfg.AI.EmbedProvider != "anthropic" {
		return fmt.Errorf("AI_EMBED_PROVIDER must be one of: openai, anthropic")
	}
	return nil
}

// 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isPlaceholderValue    = secretstrength.IsPlaceholderValue
	oauthConfigValueReady = secretstrength.OAuthConfigValueReady
)

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
