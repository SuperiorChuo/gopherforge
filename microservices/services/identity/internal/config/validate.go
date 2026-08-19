package config

import (
	"fmt"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
	"strings"
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
		// Collect every secret problem before failing so an operator fixes the
		// whole set in one pass instead of one restart per issue.
		issues := make([]string, 0, 3)
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
		if isWeakCredential(cfg.Database.Password) {
			issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if isWeakCredential(cfg.Redis.Password) {
			issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if len(issues) > 0 {
			return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
		}
	}
	return nil
} // 凭证校验函数已迁移至 shared/pkg/secretstrength，此处保留薄包装以兼容调用方。

var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
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
