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

// enabledSMTPEmail 返回一个真会发信并且带 AUTH 的邮件通道——只有这种形态才
// 校验 EMAIL_SMTP_PASSWORD。
func enabledSMTPEmail(password string) EmailConfig {
	return EmailConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		Username: "test-smtp-user@example.com",
		Password: password,
		Sender:   "test-smtp-user@example.com",
	}
}

// 邮件通道启用且要走 AUTH 时，弱密码没有安全降级语义：密码会明文发到远端
// SMTP 服务器，弱值必须在启动期拦下。
func TestValidateRejectsWeakSMTPPasswordInProduction(t *testing.T) {
	for name, password := range map[string]string{
		"未设置":   "",
		"开发默认值": "123456",
		"通用词":   "password",
		"占位符":   "your-password",
		"待替换":   "change-me",
		"开发前缀":  "dev-smtp-password",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Notification.Email = enabledSMTPEmail(password)
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "EMAIL_SMTP_PASSWORD") {
				t.Fatalf("validate() error = %v, want EMAIL_SMTP_PASSWORD rejection", err)
			}
		})
	}
}

// 关键用例：通道关闭时一律不校验。生产环境里 EMAIL_SMTP_HOST 留空（通道关闭）
// 是常态，任何无条件硬拦都会让服务重启后起不来。
func TestValidateAllowsWeakSMTPPasswordWhenEmailChannelDisabled(t *testing.T) {
	for name, mutate := range map[string]func(*EmailConfig){
		"开关关闭":         func(email *EmailConfig) { email.Enabled = false },
		"host 留空":      func(email *EmailConfig) { email.SMTPHost = "" },
		"开关关闭且 host 空": func(email *EmailConfig) { email.Enabled = false; email.SMTPHost = "" },
	} {
		t.Run(name, func(t *testing.T) {
			for _, password := range []string{"", "123456"} {
				cfg := prodConfig()
				email := enabledSMTPEmail(password)
				mutate(&email)
				cfg.Notification.Email = email
				if err := validate(cfg); err != nil {
					t.Fatalf("validate() error = %v, want nil while the email channel is off", err)
				}
			}
		})
	}
}

// 匿名转发（用户名与密码都不配）是合法形态：smtpAuth 此时根本不发 AUTH 命令，
// 没有凭据可校验，不能因为"密码为空"把服务拦在启动期。
func TestValidateAllowsAnonymousSMTPRelayInProduction(t *testing.T) {
	cfg := prodConfig()
	email := enabledSMTPEmail("")
	email.Username = ""
	cfg.Notification.Email = email
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() error = %v, want nil for an anonymous SMTP relay", err)
	}
}

// 只配了密码没配用户名也算走 AUTH（smtpAuth 只要有一项非空就发 AUTH）。
func TestValidateRejectsWeakSMTPPasswordWithoutUsernameInProduction(t *testing.T) {
	cfg := prodConfig()
	email := enabledSMTPEmail("123456")
	email.Username = ""
	cfg.Notification.Email = email
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "EMAIL_SMTP_PASSWORD") {
		t.Fatalf("validate() error = %v, want EMAIL_SMTP_PASSWORD rejection", err)
	}
}

func TestValidateAcceptsStrongSMTPPasswordInProduction(t *testing.T) {
	cfg := prodConfig()
	cfg.Notification.Email = enabledSMTPEmail("test-smtp-password-for-unit-tests")
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() error = %v, want nil for a strong SMTP password", err)
	}
}

// Load 路径回归：通道开着配弱密码，启动期就要拦下。
func TestLoadRejectsWeakSMTPPasswordFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	t.Setenv("EMAIL_NOTIFICATION_ENABLED", "true")
	t.Setenv("EMAIL_SMTP_HOST", "smtp.example.com")
	t.Setenv("EMAIL_SMTP_USERNAME", "test-smtp-user@example.com")
	t.Setenv("EMAIL_SMTP_PASSWORD", "123456")
	if err := Load(); err == nil || !strings.Contains(err.Error(), "EMAIL_SMTP_PASSWORD") {
		t.Fatalf("Load() error = %v, want EMAIL_SMTP_PASSWORD rejection", err)
	}
}

// Load 路径回归（部署现状）：EMAIL_SMTP_* 全空，生产环境必须照常启动。
func TestLoadAllowsEmptySMTPConfigFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	t.Setenv("EMAIL_SMTP_HOST", "")
	t.Setenv("EMAIL_SMTP_USERNAME", "")
	t.Setenv("EMAIL_SMTP_PASSWORD", "")
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v, want nil while the email channel is unconfigured", err)
	}
}
