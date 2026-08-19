package config

import (
	"fmt"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
	"net/url"
	"strings"
)

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
	if Cfg.Security.PasswordMaxAgeDays < 0 {
		return fmt.Errorf("security.password_max_age_days must be greater than or equal to 0")
	}
	if Cfg.Security.PasswordHistoryCount < 0 {
		return fmt.Errorf("security.password_history_count must be greater than or equal to 0")
	}
	if Cfg.Notification.Email.UseTLS && Cfg.Notification.Email.StartTLS {
		return fmt.Errorf("notification.email.use_tls and notification.email.start_tls cannot both be true")
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
	// IsSMTPAuthUnsafe re-tests the environment itself, which is redundant here
	// (Validate only calls this function in production) on purpose: routing the
	// startup gate through the exported predicate is what keeps this check and
	// the runtime config layer's fail-closed guard on one single definition.
	if IsSMTPAuthUnsafe(config.App.Env, config.Notification.Email) {
		issues = append(issues, "notification.email.password must not be empty, default, weak, or placeholder while SMTP authentication is configured")
	}

	if len(issues) > 0 {
		return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
	}
	return nil
}

// smtpAuthConfigured reports whether the email channel is both switched on and
// set up to authenticate. notification.email.enabled=false or an empty
// notification.email.smtp_host means no mail is ever sent (both are copied
// straight into runtimeconfig.EmailNotification, which is what any sender
// reads), and username plus password both empty is an anonymous relay that
// sends no AUTH command — none of those shapes carries a credential worth
// blocking startup over. Only the remaining shape has a password that would
// travel to a remote SMTP server.
func smtpAuthConfigured(email EmailConfig) bool {
	if !email.Enabled || strings.TrimSpace(email.SMTPHost) == "" {
		return false
	}
	return strings.TrimSpace(email.Username) != "" || strings.TrimSpace(email.Password) != ""
}

// IsSMTPAuthUnsafe reports whether authenticating against an SMTP server with
// this email configuration is unsafe in env. It is the one definition of that
// policy: outside production nothing is refused (local development stays
// zero-config), a channel that never sends AUTH carries no credential to judge
// (smtpAuthConfigured), and only the remaining shape puts its password up
// against isWeakCredential.
//
// Exported for internal/pkg/runtimeconfig, where a system_settings row can
// switch notification.email on long after Validate inspected the file and
// environment derived settings — that layer has to fail closed on exactly the
// shape refused here. Sharing this predicate instead of exporting
// isProductionEnv and isWeakCredential separately is deliberate: it makes the
// two gates impossible to drift apart and keeps the raw credential primitives
// out of reach of unrelated callers.
func IsSMTPAuthUnsafe(env string, email EmailConfig) bool {
	return isProductionEnv(env) && smtpAuthConfigured(email) && isWeakCredential(email.Password)
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

// Credential-check helpers moved to shared/pkg/secretstrength; thin wrappers
// kept here for call-site compatibility.
var (
	isProductionEnv       = secretstrength.IsProductionEnv
	isStrongSecret        = secretstrength.IsStrongSecret
	isWeakCredential      = secretstrength.IsWeakCredential
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
