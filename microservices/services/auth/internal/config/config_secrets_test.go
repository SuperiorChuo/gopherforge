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

// enabledOAuthProvider 返回一个 Ready() 为真的 provider——也就是 OAuth 服务
// 真会拿去出网的形态，只有这种形态才校验 client_secret。
func enabledOAuthProvider(clientSecret string) OAuthProviderConfig {
	return OAuthProviderConfig{
		Enabled:      true,
		ClientID:     "test-oauth-client-id",
		ClientSecret: clientSecret,
		RedirectURI:  "https://example.com/api/v1/oauth/callback",
	}
}

// client_secret 的弱值原先能通过 oauthConfigValueReady——它只拦占位符，
// 不查弱值表，于是 GITHUB_CLIENT_SECRET=123456 会被判定为"就绪"并启用 OAuth。
func TestValidateRejectsWeakGithubClientSecretInProduction(t *testing.T) {
	for name, secret := range map[string]string{
		"开发默认值": "123456",
		"通用词":   "secret",
		"项目名":   "go-admin-kit",
		"开发前缀":  "dev-github-client-secret",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.OAuth.Github = enabledOAuthProvider(secret)
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "GITHUB_CLIENT_SECRET") {
				t.Fatalf("validate() error = %v, want GITHUB_CLIENT_SECRET rejection", err)
			}
		})
	}
}

// 两个 provider 各自独立校验，报错要指名到具体环境变量。
func TestValidateRejectsWeakWechatClientSecretInProduction(t *testing.T) {
	cfg := prodConfig()
	cfg.OAuth.Wechat = enabledOAuthProvider("123456")
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "WECHAT_CLIENT_SECRET") {
		t.Fatalf("validate() error = %v, want WECHAT_CLIENT_SECRET rejection", err)
	}
	if strings.Contains(err.Error(), "GITHUB_CLIENT_SECRET") {
		t.Fatalf("validate() error = %v, want no GITHUB_CLIENT_SECRET issue for a disabled provider", err)
	}
}

// 关键用例：OAuth 未启用时一律不校验。生产环境里 *_OAUTH_ENABLED 默认关闭且
// client_secret 为空是常态，任何无条件硬拦都会让服务重启后起不来。
func TestValidateAllowsWeakOAuthClientSecretWhenProviderDisabled(t *testing.T) {
	for name, secret := range map[string]string{
		"未设置":   "",
		"开发默认值": "123456",
		"占位符":   "your-github-client-secret",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.OAuth.Github = enabledOAuthProvider(secret)
			cfg.OAuth.Github.Enabled = false
			cfg.OAuth.Wechat = enabledOAuthProvider(secret)
			cfg.OAuth.Wechat.Enabled = false
			if err := validate(cfg); err != nil {
				t.Fatalf("validate() error = %v, want nil while both providers are disabled", err)
			}
		})
	}
}

// 开关开着但三件套没齐 → Ready() 为假 → provider 运行期本就返回不可用，
// 校验同样不介入，免得半配状态把服务拦在启动期。
func TestValidateAllowsIncompleteOAuthProviderInProduction(t *testing.T) {
	strongSecret := "test-github-client-secret-for-unit-tests"
	for name, mutate := range map[string]func(*OAuthProviderConfig){
		"secret 未配":       func(p *OAuthProviderConfig) { p.ClientSecret = "" },
		"secret 仍是占位符":    func(p *OAuthProviderConfig) { p.ClientSecret = "change-me" },
		"client_id 未配":    func(p *OAuthProviderConfig) { p.ClientID = "" },
		"redirect_uri 未配": func(p *OAuthProviderConfig) { p.RedirectURI = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := prodConfig()
			provider := enabledOAuthProvider(strongSecret)
			mutate(&provider)
			cfg.OAuth.Github = provider
			if provider.Ready() {
				t.Fatalf("fixture Ready() = true, want an incomplete provider")
			}
			if err := validate(cfg); err != nil {
				t.Fatalf("validate() error = %v, want nil for an incomplete provider", err)
			}
		})
	}
}

func TestValidateAcceptsStrongOAuthClientSecretInProduction(t *testing.T) {
	cfg := prodConfig()
	cfg.OAuth.Github = enabledOAuthProvider("test-github-client-secret-for-unit-tests")
	cfg.OAuth.Wechat = enabledOAuthProvider("test-wechat-client-secret-for-unit-tests")
	if !cfg.OAuth.Github.Ready() || !cfg.OAuth.Wechat.Ready() {
		t.Fatal("fixture Ready() = false, want both providers enabled")
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() error = %v, want nil for strong client secrets", err)
	}
}

// Load 路径回归：开关开着配弱 secret，启动期就要拦下。
func TestLoadRejectsWeakGithubClientSecretFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	t.Setenv("GITHUB_OAUTH_ENABLED", "true")
	t.Setenv("GITHUB_CLIENT_ID", "test-github-client-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "123456")
	t.Setenv("GITHUB_REDIRECT_URI", "https://example.com/api/v1/oauth/github/callback")
	if err := Load(); err == nil || !strings.Contains(err.Error(), "GITHUB_CLIENT_SECRET") {
		t.Fatalf("Load() error = %v, want GITHUB_CLIENT_SECRET rejection", err)
	}
}

// Load 路径回归（部署现状）：开关关闭且 secret 留空，生产环境必须照常启动。
func TestLoadAllowsDisabledOAuthWithWeakSecretFromEnv(t *testing.T) {
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "test-jwt-secret-for-unit-tests-0123456789")
	t.Setenv("DB_PASSWORD", "test-db-password-for-unit-tests")
	t.Setenv("REDIS_PASSWORD", "test-redis-password-for-unit-tests")
	t.Setenv("GITHUB_OAUTH_ENABLED", "false")
	t.Setenv("GITHUB_CLIENT_SECRET", "123456")
	t.Setenv("WECHAT_OAUTH_ENABLED", "false")
	t.Setenv("WECHAT_CLIENT_SECRET", "")
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v, want nil while OAuth is switched off", err)
	}
}
