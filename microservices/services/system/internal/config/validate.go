package config

import (
	"encoding/base64"
	"fmt"
	secretstrength "github.com/go-admin-kit/services/shared/pkg/secretstrength"
	"os"
	"path/filepath"
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
	if cfg.Codegen.WriteEnabled {
		if strings.TrimSpace(cfg.App.Env) != "development" {
			return fmt.Errorf("CODEGEN_WRITE_ENABLED requires APP_ENV=development")
		}
		if err := validateCodegenRepoRoot(cfg.Codegen.RepoRoot); err != nil {
			return err
		}
	}
	if err := validateEdgeCertConfig(cfg.EdgeCert); err != nil {
		return err
	}
	if isProductionEnv(cfg.App.Env) {
		// 在失败前收集所有密钥问题，使运维可以一次修完整套配置，
		// 而不是每个问题重启一次。
		issues := make([]string, 0, 5)
		if !isStrongSecret(cfg.JWT.Secret, 32) {
			issues = append(issues, "JWT_SECRET must be at least 32 characters and must not use a default or placeholder value")
		}
		if isWeakCredential(cfg.Database.Password) {
			issues = append(issues, "DB_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if isWeakCredential(cfg.Redis.Password) {
			issues = append(issues, "REDIS_PASSWORD must not be empty, default, weak, or placeholder")
		}
		if cfg.InternalToken != "" && isWeakCredential(cfg.InternalToken) {
			issues = append(issues, "SYSTEM_INTERNAL_TOKEN must not use a default, weak, or placeholder value")
		}
		// 生产环境允许“只查看外部托管 TLS”的零密钥形态；但一旦声明
		// current key id，密钥必须同时存在且可解码。Issue/Export 还会独立 fail closed。
		if cfg.EdgeCert.CurrentKeyID != "" && cfg.EdgeCert.CurrentKeyBase64 == "" {
			issues = append(issues, "EDGE_CERT_ENCRYPTION_KEY is required when EDGE_CERT_CURRENT_KEY_ID is set")
		}
		// IsSMTPAuthUnsafe 会重新读取环境本身，在该代码块内看似冗余，
		// 但这是刻意为之：让启动门禁走这个导出的谓词，才能保证这里的检查
		// 与运行时配置层的 fail-closed 守卫始终基于同一份定义。
		if IsSMTPAuthUnsafe(cfg.App.Env, cfg.Notification.Email) {
			issues = append(issues, "EMAIL_SMTP_PASSWORD must not be empty, default, weak, or placeholder while SMTP authentication is configured")
		}
		if len(issues) > 0 {
			return fmt.Errorf("production safety checks failed: %s", strings.Join(issues, "; "))
		}
	}
	return nil
}

func validateEdgeCertConfig(cfg EdgeCertConfig) error {
	if cfg.RenewBeforeDays <= 0 || cfg.RenewBeforeDays > 60 {
		return fmt.Errorf("EDGE_CERT_RENEW_BEFORE_DAYS must be between 1 and 60")
	}
	if cfg.TaskPollSeconds <= 0 || cfg.TaskPollSeconds > 60 {
		return fmt.Errorf("EDGE_CERT_TASK_POLL_SECONDS must be between 1 and 60")
	}
	if cfg.ChallengeTTLMinutes < 5 || cfg.ChallengeTTLMinutes > 60 {
		return fmt.Errorf("EDGE_CERT_CHALLENGE_TTL_MINUTES must be between 5 and 60")
	}
	if (cfg.PreviousKeyID == "") != (cfg.PreviousKeyBase64 == "") {
		return fmt.Errorf("EDGE_CERT_PREVIOUS_KEY_ID and EDGE_CERT_PREVIOUS_ENCRYPTION_KEY must be configured together")
	}
	if (cfg.CurrentKeyID == "") != (cfg.CurrentKeyBase64 == "") {
		return fmt.Errorf("EDGE_CERT_CURRENT_KEY_ID and EDGE_CERT_ENCRYPTION_KEY must be configured together")
	}
	if cfg.CurrentKeyID != "" && cfg.CurrentKeyID == cfg.PreviousKeyID {
		return fmt.Errorf("EDGE_CERT_CURRENT_KEY_ID and EDGE_CERT_PREVIOUS_KEY_ID must differ")
	}
	if cfg.StorageRoot == "" && cfg.TraefikDynamicDir != "" {
		return fmt.Errorf("EDGE_CERT_STORAGE_ROOT is required when EDGE_CERT_TRAEFIK_DYNAMIC_DIR is set")
	}
	if _, _, _, _, err := cfg.KeyMaterials(); err != nil {
		return err
	}
	return nil
}

func decodeEdgeCertKey(name, encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("%s must be standard base64", name)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("%s must decode to exactly 32 bytes", name)
	}
	return decoded, nil
}

// smtpAuthConfigured 报告邮件通道是否真的会对 SMTP 服务器进行认证。
// 当 Enabled 为 false 时 mailer.SMTPSender.Send 会提前返回，并拒绝空主机；
// 当用户名和密码都为空时，mailer 的 smtpAuth 不会发送任何 AUTH 命令——
// 因此关闭的通道与匿名中继都不携带值得阻塞启动的凭据。只有剩下的一种形态
// （通道开启、主机已设置、使用 AUTH）才会让 EMAIL_SMTP_PASSWORD 流向远程服务器。
func smtpAuthConfigured(email EmailConfig) bool {
	if !email.Enabled || strings.TrimSpace(email.SMTPHost) == "" {
		return false
	}
	return strings.TrimSpace(email.Username) != "" || strings.TrimSpace(email.Password) != ""
}

// IsSMTPAuthUnsafe 报告在 env 环境下使用该邮件配置对 SMTP 服务器认证是否不安全。
// 它是该策略的唯一定义：生产环境之外什么都不拒绝（本地开发保持零配置），
// 从不发送 AUTH 的通道没有可判定的凭据（smtpAuthConfigured），
// 只有剩下那种形态才会把密码交给 isWeakCredential 判定。
//
// 导出给 internal/pkg/runtimeconfig 使用：在 validate() 检查完环境派生设置很久之后，
// 一条 system_settings 记录仍可能把 notification.email 打开——那一层必须
// 对这里拒绝的形态同样采取 fail-closed。刻意共享这一个谓词而非分别导出
// isProductionEnv 与 isWeakCredential，是为了让两道门禁不可能相互偏离，
// 并让原始凭据判断原语远离无关调用方。
func IsSMTPAuthUnsafe(env string, email EmailConfig) bool {
	return isProductionEnv(env) && smtpAuthConfigured(email) && isWeakCredential(email.Password)
}

func validateCodegenRepoRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("CODEGEN_REPO_ROOT is required when repository write is enabled")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("CODEGEN_REPO_ROOT is invalid")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("CODEGEN_REPO_ROOT is invalid")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("CODEGEN_REPO_ROOT is not a directory")
	}
	for _, required := range []string{
		".git",
		filepath.Join("microservices", "services", "system"),
		filepath.Join("microservices", "web"),
	} {
		if _, err := os.Stat(filepath.Join(canonical, required)); err != nil {
			return fmt.Errorf("CODEGEN_REPO_ROOT is missing required repository paths")
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
