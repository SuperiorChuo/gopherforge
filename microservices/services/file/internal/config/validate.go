package config

import (
	"fmt"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
	"net/url"
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
	switch cfg.Upload.EffectiveStorageType() {
	case "local", "s3", "minio":
	default:
		return fmt.Errorf("UPLOAD_STORAGE_TYPE must be one of: local, s3, minio")
	}
	if isProductionEnv(cfg.App.Env) {
		// Collect every secret problem before failing so an operator fixes the
		// whole set in one pass instead of one restart per issue.
		issues := make([]string, 0, 4)
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
		if isWeakCredential(cfg.Database.Password) {
			issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if isWeakCredential(cfg.Redis.Password) {
			issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
		}
		// Object storage credentials only exist for the selected backend: local
		// disk has none, so the checks stay scoped to the effective storage type
		// instead of demanding S3/MinIO settings from every deployment.
		switch cfg.Upload.EffectiveStorageType() {
		case "s3":
			issues = appendObjectStorageIssues(issues, "UPLOAD_S3", cfg.Upload.S3, true)
		case "minio":
			issues = appendObjectStorageIssues(issues, "UPLOAD_MINIO", cfg.Upload.MinIO, false)
		}
		if len(issues) > 0 {
			return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
		}
	}
	return nil
}

// appendObjectStorageIssues promotes the required-field set that
// internal/pkg/upload.validateObjectStorageConfig already enforces lazily
// (endpoint, bucket, region for s3 only, access key, secret key) to a startup
// check, and additionally rejects credentials that are weak rather than merely
// empty. Region stays optional for MinIO, matching the storage client.
func appendObjectStorageIssues(issues []string, envPrefix string, storage ObjectStorageConfig, requireRegion bool) []string {
	issues = appendObjectStorageEndpointIssues(issues, envPrefix, storage.Endpoint)
	if strings.TrimSpace(storage.Bucket) == "" {
		issues = append(issues, envPrefix+"_BUCKET must be set")
	}
	if requireRegion && strings.TrimSpace(storage.Region) == "" {
		issues = append(issues, envPrefix+"_REGION must be set")
	}
	if isWeakCredential(storage.AccessKey) {
		issues = append(issues, envPrefix+"_ACCESS_KEY must not be empty, default, weak, or placeholder")
	}
	if isWeakCredential(storage.SecretKey) {
		issues = append(issues, envPrefix+"_SECRET_KEY must not be empty, default, weak, or placeholder")
	}
	return issues
}

// appendObjectStorageEndpointIssues applies the same endpoint shape rules as
// internal/pkg/upload.objectStorageEndpoint so a malformed endpoint fails at
// boot instead of on the first upload.
func appendObjectStorageEndpointIssues(issues []string, envPrefix string, endpoint string) []string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return append(issues, envPrefix+"_ENDPOINT must be set")
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			return append(issues, envPrefix+"_ENDPOINT must be a valid host or URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return append(issues, envPrefix+"_ENDPOINT must use http or https")
		}
		if strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return append(issues, envPrefix+"_ENDPOINT must not include path, query, or fragment")
		}
		return issues
	}
	if strings.ContainsAny(endpoint, "/\\?#") {
		return append(issues, envPrefix+"_ENDPOINT must not include path, query, or fragment")
	}
	return issues
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
