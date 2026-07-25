package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prodSafetyConfig 生产环境下的合规基线。monitor 没有 Defaults()（配置来自
// configs/config.yaml），所以夹具手工搭，只填生产校验会看的字段；
// 值用显式的 test- 前缀，既不像真凭据也不落进弱值黑名单。
func prodSafetyConfig() Config {
	cfg := Config{}
	cfg.App.Env = "production"
	cfg.JWT.Secret = "test-jwt-secret-for-unit-tests-0123456789"
	cfg.Database.Password = "test-db-password-for-unit-tests"
	cfg.Redis.Password = "test-redis-password-for-unit-tests"
	return cfg
}

func TestValidateProductionSafetyAcceptsConfiguredConfig(t *testing.T) {
	if err := validateProductionSafety(prodSafetyConfig()); err != nil {
		t.Fatalf("validateProductionSafety() error = %v, want nil for a configured production config", err)
	}
}

// enabledSMTPEmail 返回一个真会发信并且带 AUTH 的邮件通道——只有这种形态才
// 校验 notification.email.password。
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

// 邮件通道启用且要走 AUTH 时，弱密码没有安全降级语义：密码会发到远端 SMTP
// 服务器，弱值必须在启动期拦下。
func TestValidateProductionSafetyRejectsWeakSMTPPassword(t *testing.T) {
	for name, password := range map[string]string{
		"未设置":   "",
		"开发默认值": "123456",
		"通用词":   "password",
		"占位符":   "your-password",
		"待替换":   "change-me",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodSafetyConfig()
			cfg.Notification.Email = enabledSMTPEmail(password)
			err := validateProductionSafety(cfg)
			if err == nil || !strings.Contains(err.Error(), "notification.email.password") {
				t.Fatalf("validateProductionSafety() error = %v, want notification.email.password rejection", err)
			}
		})
	}
}

// 关键用例：通道关闭时一律不校验。configs/config.yaml 的缺省形态就是
// enabled=false + smtp_host 留空，任何无条件硬拦都会让服务重启后起不来。
func TestValidateProductionSafetyAllowsWeakSMTPPasswordWhenChannelDisabled(t *testing.T) {
	for name, mutate := range map[string]func(*EmailConfig){
		"开关关闭":         func(email *EmailConfig) { email.Enabled = false },
		"host 留空":      func(email *EmailConfig) { email.SMTPHost = "" },
		"开关关闭且 host 空": func(email *EmailConfig) { email.Enabled = false; email.SMTPHost = "" },
	} {
		t.Run(name, func(t *testing.T) {
			for _, password := range []string{"", "123456"} {
				cfg := prodSafetyConfig()
				email := enabledSMTPEmail(password)
				mutate(&email)
				cfg.Notification.Email = email
				if err := validateProductionSafety(cfg); err != nil {
					t.Fatalf("validateProductionSafety() error = %v, want nil while the email channel is off", err)
				}
			}
		})
	}
}

// 匿名转发（用户名与密码都不配）是合法形态：这种配置下根本不发 AUTH 命令，
// 没有凭据可校验，不能因为"密码为空"把服务拦在启动期。
func TestValidateProductionSafetyAllowsAnonymousSMTPRelay(t *testing.T) {
	cfg := prodSafetyConfig()
	email := enabledSMTPEmail("")
	email.Username = ""
	cfg.Notification.Email = email
	if err := validateProductionSafety(cfg); err != nil {
		t.Fatalf("validateProductionSafety() error = %v, want nil for an anonymous SMTP relay", err)
	}
}

// 只配了密码没配用户名也算走 AUTH（有一项非空就会发 AUTH）。
func TestValidateProductionSafetyRejectsWeakSMTPPasswordWithoutUsername(t *testing.T) {
	cfg := prodSafetyConfig()
	email := enabledSMTPEmail("123456")
	email.Username = ""
	cfg.Notification.Email = email
	err := validateProductionSafety(cfg)
	if err == nil || !strings.Contains(err.Error(), "notification.email.password") {
		t.Fatalf("validateProductionSafety() error = %v, want notification.email.password rejection", err)
	}
}

func TestValidateProductionSafetyAcceptsStrongSMTPPassword(t *testing.T) {
	cfg := prodSafetyConfig()
	cfg.Notification.Email = enabledSMTPEmail("test-smtp-password-for-unit-tests")
	if err := validateProductionSafety(cfg); err != nil {
		t.Fatalf("validateProductionSafety() error = %v, want nil for a strong SMTP password", err)
	}
}

// 非生产环境不校验：本地零配置要能直接跑起来。
func TestValidateAllowsWeakSMTPPasswordOutsideProduction(t *testing.T) {
	oldCfg := Cfg
	t.Cleanup(func() { Cfg = oldCfg })
	for _, env := range []string{"development", "staging", ""} {
		Cfg = Config{}
		Cfg.App.Env = env
		Cfg.Notification.Email = enabledSMTPEmail("123456")
		if err := Validate(); err != nil {
			t.Fatalf("Validate() error = %v for app.env=%q, want nil outside production", err, env)
		}
	}
}

// IsSMTPAuthUnsafe 是启动期校验与 runtimeconfig 热配置共用的唯一判定：
// 这里把它的各个分支单独钉住，热配置那侧只要调它就不会与启动期口径漂移。
func TestIsSMTPAuthUnsafeCoversStartupCriteria(t *testing.T) {
	anonymous := enabledSMTPEmail("")
	anonymous.Username = ""
	channelOff := enabledSMTPEmail("123456")
	channelOff.Enabled = false
	hostless := enabledSMTPEmail("123456")
	hostless.SMTPHost = ""

	for name, testCase := range map[string]struct {
		env   string
		email EmailConfig
		want  bool
	}{
		"生产 + 走 AUTH + 弱密码": {env: "production", email: enabledSMTPEmail("123456"), want: true},
		"生产 + 走 AUTH + 强密码": {env: "production", email: enabledSMTPEmail("test-smtp-password-for-unit-tests"), want: false},
		"生产 + 匿名转发":         {env: "production", email: anonymous, want: false},
		"生产 + 通道关闭":         {env: "production", email: channelOff, want: false},
		"生产 + host 留空":      {env: "production", email: hostless, want: false},
		"开发环境 + 弱密码":        {env: "development", email: enabledSMTPEmail("123456"), want: false},
		"预发环境 + 弱密码":        {env: "staging", email: enabledSMTPEmail("123456"), want: false},
		"app.env 未设置 + 弱密码": {env: "", email: enabledSMTPEmail("123456"), want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := IsSMTPAuthUnsafe(testCase.env, testCase.email); got != testCase.want {
				t.Fatalf("IsSMTPAuthUnsafe() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// 弱值口径与 auth / audit / file / identity / system 五个底座服务对齐：monitor 此前
// 缺裸 secret 与 dev- 前缀兜底，同一个占位口令在别处被拦、在这里放行。
func TestIsWeakCredentialMatchesPeerServices(t *testing.T) {
	for name, value := range map[string]string{
		"裸 secret":        "secret",
		"裸 secret 大小写":    "Secret",
		"裸 secret 带空白":    "  secret  ",
		"dev- 前缀":         "dev-monitor-password",
		"dev- 前缀大小写":      "DEV-Monitor-Password",
		"既有条目 secret-key": "secret-key",
	} {
		t.Run(name, func(t *testing.T) {
			if !isWeakCredential(value) {
				t.Fatalf("isWeakCredential(%q) = false, want true to match the peer services", value)
			}
		})
	}
}

// 反向对照：随机强值不能被新规则误伤。真实部署的 DB / Redis / 对象存储凭据都是随机串，
// 这里用等长的 test- 占位模拟（真值不读取、不打印），确认没有一条被判弱。
func TestIsWeakCredentialAllowsRandomStrongValues(t *testing.T) {
	for name, value := range map[string]string{
		"13 位 DB 口令":      "test-db-pwd13",
		"18 位 Redis 口令":   "test-redis-pwd18ab",
		"9 位对象存储 access":  "test-min9",
		"32 位对象存储 secret": "test-minio-secret-key-32chars-ab",
		"含 secret 子串":     "test-secret-but-random",
		"含 dev 子串但非前缀":    "test-dev-random-value",
	} {
		t.Run(name, func(t *testing.T) {
			if isWeakCredential(value) {
				t.Fatalf("isWeakCredential(%q) = true, want false for a random production credential", value)
			}
		})
	}
}

// LoadConfig + Validate 全路径回归：yaml 里通道开着配弱密码，启动期就要拦下。
func TestValidateRejectsWeakSMTPPasswordFromYAML(t *testing.T) {
	oldCfg := Cfg
	t.Cleanup(func() { Cfg = oldCfg })
	// 这几项由 replaceEnvVars 覆盖 yaml，显式钉住以免继承外部环境变量。
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`notification:
  email:
    enabled: true
    smtp_host: "smtp.example.com"
    smtp_port: 587
    username: "test-smtp-user@example.com"
    password: "123456"
`)
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	Cfg = Config{}
	if err := LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	err := Validate()
	if err == nil || !strings.Contains(err.Error(), "notification.email.password") {
		t.Fatalf("Validate() error = %v, want notification.email.password rejection", err)
	}
}

// LoadConfig + Validate 全路径回归（部署现状）：yaml 里通道关闭、smtp_host 留空，
// 生产环境必须照常启动。
func TestValidateAllowsDisabledEmailChannelFromYAML(t *testing.T) {
	oldCfg := Cfg
	t.Cleanup(func() { Cfg = oldCfg })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`notification:
  email:
    enabled: false
    smtp_host: ""
    username: ""
    password: ""
`)
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	Cfg = Config{}
	if err := LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if err := Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil while the email channel is off", err)
	}
}
