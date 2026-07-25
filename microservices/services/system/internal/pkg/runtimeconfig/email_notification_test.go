package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/system/internal/config"
	"github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

// 夹具全部用显式的 test- 前缀值：既不像真凭据（不会撞 pre-commit 的密钥扫描），
// 也不落进弱值黑名单（弱值是精确匹配，"test" 算弱但 "test-xxx" 不算）。
const (
	testStrongSMTPPassword = "test-smtp-password-for-unit-tests"
	testSMTPUsername       = "test-smtp-user@example.com"
)

// weakAuthEmailConfig 是"绕过启动期校验"的关键形态：env 里通道是关的（所以
// config.validate 放行了弱密码），但密码就摆在那里，等着热配置把通道打开。
func weakAuthEmailConfig(password string) config.EmailConfig {
	return config.EmailConfig{
		Enabled:  false,
		SMTPHost: "",
		SMTPPort: 587,
		Username: testSMTPUsername,
		Password: password,
		Sender:   testSMTPUsername,
	}
}

// useEmailConfig 钉住 APP_ENV 与 env 派生的邮件配置，退出时整体还原。
func useEmailConfig(t *testing.T, env string, email config.EmailConfig) {
	t.Helper()
	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })
	config.Cfg.App.Env = env
	config.Cfg.Notification.Email = email
}

// captureEmailNotificationWarnings 把 fail closed 的告警收进切片，免得为了断言
// "有 WARNING"去初始化全局 logger。
func captureEmailNotificationWarnings(t *testing.T) *[]EmailNotification {
	t.Helper()
	previous := warnEmailNotificationKeptDisabled
	warnings := make([]EmailNotification, 0, 2)
	warnEmailNotificationKeptDisabled = func(policy EmailNotification) {
		warnings = append(warnings, policy)
	}
	t.Cleanup(func() { warnEmailNotificationKeptDisabled = previous })
	return &warnings
}

func emailSettingReader(value map[string]any) *CachedEmailNotificationReader {
	return NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON:  value,
	}}, time.Minute)
}

// 核心用例：控制台把 enabled 改成 true（并自带 smtp_host），而 EMAIL_SMTP_PASSWORD
// 是弱值。启动期校验当时看到的是 Enabled=false 所以放行了，运行期必须 fail closed
// ——通道保持关闭并打一条 WARNING，而不是带着弱密码去连远端 SMTP。
func TestEmailNotificationKeepsChannelClosedWhenSettingEnablesWeakSMTPAuthInProduction(t *testing.T) {
	for name, password := range map[string]string{
		"开发默认值": "123456",
		"通用词":   "password",
		"占位符":   "your-password",
		"待替换":   "change-me",
		"开发前缀":  "dev-smtp-password",
		"未设置":   "",
	} {
		t.Run(name, func(t *testing.T) {
			email := weakAuthEmailConfig(password)
			if password == "" {
				// 密码空但用户名非空依然会发 AUTH，属于要拦的形态。
				email.Username = testSMTPUsername
			}
			useEmailConfig(t, "production", email)
			warnings := captureEmailNotificationWarnings(t)

			policy := emailSettingReader(map[string]any{
				"enabled":   true,
				"smtp_host": "smtp.example.com",
			}).EmailNotification(context.Background())

			if policy.Enabled {
				t.Fatal("email channel should stay closed when production SMTP AUTH would use a weak password")
			}
			if len(*warnings) != 1 {
				t.Fatalf("warnings = %d, want exactly one WARNING explaining the refusal", len(*warnings))
			}
			if got := (*warnings)[0].SMTPHost; got != "smtp.example.com" {
				t.Fatalf("warning smtp_host = %q, want the host that would have been dialed", got)
			}
			if policy.SMTPHost != "smtp.example.com" {
				t.Fatalf("smtp host = %q, want the runtime override still applied", policy.SMTPHost)
			}
		})
	}
}

// 强密码时热配置照常生效：这道闸只针对弱凭据，不是把热开关废掉。
func TestEmailNotificationEnablesChannelWithStrongSMTPPasswordInProduction(t *testing.T) {
	useEmailConfig(t, "production", weakAuthEmailConfig(testStrongSMTPPassword))
	warnings := captureEmailNotificationWarnings(t)

	policy := emailSettingReader(map[string]any{
		"enabled":   true,
		"smtp_host": "smtp.example.com",
	}).EmailNotification(context.Background())

	if !policy.Enabled {
		t.Fatal("email channel should be enabled by the runtime setting when the SMTP password is strong")
	}
	if len(*warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for a strong SMTP password", *warnings)
	}
}

// 关键用例：匿名转发（用户名与密码都空）是合法部署形态——mailer 的 smtpAuth 此时
// 根本不发 AUTH 命令，没有凭据可泄露，不能被当成不安全而关掉通道。
func TestEmailNotificationEnablesAnonymousRelayInProduction(t *testing.T) {
	email := weakAuthEmailConfig("")
	email.Username = ""
	useEmailConfig(t, "production", email)
	warnings := captureEmailNotificationWarnings(t)

	policy := emailSettingReader(map[string]any{
		"enabled":   true,
		"smtp_host": "smtp.example.com",
	}).EmailNotification(context.Background())

	if !policy.Enabled {
		t.Fatal("an anonymous SMTP relay is a legitimate deployment and must stay enabled")
	}
	if len(*warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for an anonymous relay", *warnings)
	}
}

// 判定条件与启动期校验同源：smtp_host 留空时不算"会发 AUTH"，所以不拦
// （mailer 自己会拒绝空 host）。这正是通道未配置时的部署形态。
func TestEmailNotificationLeavesHostlessChannelAloneInProduction(t *testing.T) {
	useEmailConfig(t, "production", weakAuthEmailConfig("123456"))
	warnings := captureEmailNotificationWarnings(t)

	policy := emailSettingReader(map[string]any{"enabled": true}).EmailNotification(context.Background())

	if !policy.Enabled {
		t.Fatal("a channel without smtp_host sends no AUTH and must not be forced closed")
	}
	if len(*warnings) != 0 {
		t.Fatalf("warnings = %#v, want none while smtp_host is empty", *warnings)
	}
}

// 非生产环境不做任何拦截：本地开发用 123456 连 mailhog 必须照常可用。
func TestEmailNotificationEnablesWeakSMTPAuthOutsideProduction(t *testing.T) {
	for _, env := range []string{"development", "staging", ""} {
		useEmailConfig(t, env, weakAuthEmailConfig("123456"))
		warnings := captureEmailNotificationWarnings(t)

		policy := emailSettingReader(map[string]any{
			"enabled":   true,
			"smtp_host": "smtp.example.com",
		}).EmailNotification(context.Background())

		if !policy.Enabled {
			t.Fatalf("APP_ENV=%q: email channel should stay enabled outside production", env)
		}
		if len(*warnings) != 0 {
			t.Fatalf("APP_ENV=%q: warnings = %#v, want none outside production", env, *warnings)
		}
	}
}

// 兜底路径也要堵：reader 为 nil 或取库失败且无缓存时，返回的是 env 派生策略。
// 那条路正常走不到这里（启动期就会拦下同样的形态），但闸放在 policy 的出口处，
// 三个出口一个也不漏。
func TestEmailNotificationFallbackPathsFailClosedInProduction(t *testing.T) {
	email := weakAuthEmailConfig("123456")
	email.Enabled = true
	email.SMTPHost = "smtp.example.com"

	t.Run("直接读 env 派生配置", func(t *testing.T) {
		useEmailConfig(t, "production", email)
		warnings := captureEmailNotificationWarnings(t)
		if EmailNotificationFromConfig().Enabled {
			t.Fatal("EmailNotificationFromConfig should fail closed on weak production SMTP AUTH")
		}
		if len(*warnings) != 1 {
			t.Fatalf("warnings = %d, want exactly one WARNING", len(*warnings))
		}
	})

	t.Run("reader 为 nil", func(t *testing.T) {
		useEmailConfig(t, "production", email)
		captureEmailNotificationWarnings(t)
		var reader *CachedEmailNotificationReader
		if reader.EmailNotification(context.Background()).Enabled {
			t.Fatal("nil reader fallback should fail closed on weak production SMTP AUTH")
		}
	})

	t.Run("取库失败且无缓存", func(t *testing.T) {
		useEmailConfig(t, "production", email)
		captureEmailNotificationWarnings(t)
		reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{err: errors.New("store unavailable")}, time.Minute)
		if reader.EmailNotification(context.Background()).Enabled {
			t.Fatal("store failure fallback should fail closed on weak production SMTP AUTH")
		}
	})
}

// 缓存层拿到的也必须是已经 fail closed 的策略：Refresh 之后 TTL 内的读取都命中缓存。
func TestEmailNotificationCachesFailClosedPolicy(t *testing.T) {
	useEmailConfig(t, "production", weakAuthEmailConfig("123456"))
	captureEmailNotificationWarnings(t)

	store := &stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"enabled":   true,
			"smtp_host": "smtp.example.com",
		},
	}}
	reader := NewCachedEmailNotificationReader(store, time.Minute)
	if err := reader.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if policy := reader.EmailNotification(context.Background()); policy.Enabled {
		t.Fatal("cached policy should already be the fail-closed one")
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want the cached policy to be served without a reload", store.calls)
	}
}

type stubEmailNotificationStore struct {
	setting *model.SystemSetting
	err     error
	calls   int
}

func (s *stubEmailNotificationStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.setting == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.setting, nil
}
