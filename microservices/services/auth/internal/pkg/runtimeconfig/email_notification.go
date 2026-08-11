package runtimeconfig

import (
	"context"
	"fmt"
	"sync"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

// EmailNotification holds the SMTP configuration the forgot-password link
// uses. It is read from the shared `notification.email` system_settings key
// (same source the monitor alert engine and system notices use), so operators
// configure mail once in the console. auth's container ships no EMAIL_* env,
// so the reader intentionally has no environment fallback: an unset/disabled
// setting simply means mail is skipped (the forgot-password endpoint still
// answers "sent" to avoid account enumeration).
type EmailNotification struct {
	Enabled  bool
	SMTPHost string
	SMTPPort int
	Username string
	Password string
	Sender   string
	UseTLS   bool
	StartTLS bool
}

type EmailNotificationStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

type EmailNotificationReader interface {
	EmailNotification(ctx context.Context) EmailNotification
}

const emailNotificationSettingKey = "notification.email"

type CachedEmailNotificationReader struct {
	store EmailNotificationStore
	ttl   time.Duration

	mu        sync.RWMutex
	policy    EmailNotification
	expiresAt time.Time
	loaded    bool
}

func NewCachedEmailNotificationReader(store EmailNotificationStore, ttl time.Duration) *CachedEmailNotificationReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedEmailNotificationReader{store: store, ttl: ttl}
}

var (
	emailNotificationStoreMu sync.RWMutex
	emailNotificationStore   EmailNotificationStore
)

// SetEmailNotificationStore installs the store behind
// DefaultEmailNotificationReader (the shared system_settings DAO). Idempotent;
// returns a restore func for tests.
func SetEmailNotificationStore(store EmailNotificationStore) func() {
	emailNotificationStoreMu.Lock()
	previous := emailNotificationStore
	emailNotificationStore = store
	emailNotificationStoreMu.Unlock()
	return func() {
		emailNotificationStoreMu.Lock()
		emailNotificationStore = previous
		emailNotificationStoreMu.Unlock()
	}
}

var (
	defaultEmailNotificationOnce   sync.Once
	defaultEmailNotificationReader *CachedEmailNotificationReader
)

func DefaultEmailNotificationReader() *CachedEmailNotificationReader {
	defaultEmailNotificationOnce.Do(func() {
		defaultEmailNotificationReader = NewCachedEmailNotificationReader(nil, 30*time.Second)
	})
	return defaultEmailNotificationReader
}

func (r *CachedEmailNotificationReader) EmailNotification(ctx context.Context) EmailNotification {
	if r == nil {
		return EmailNotification{}
	}
	r.mu.RLock()
	if r.loaded && time.Now().Before(r.expiresAt) {
		policy := r.policy
		r.mu.RUnlock()
		return policy
	}
	r.mu.RUnlock()

	policy := r.load(ctx)
	r.mu.Lock()
	r.policy = policy
	r.loaded = true
	r.expiresAt = time.Now().Add(r.ttl)
	r.mu.Unlock()
	return policy
}

func (r *CachedEmailNotificationReader) load(ctx context.Context) EmailNotification {
	if ctx == nil {
		ctx = context.Background()
	}
	emailNotificationStoreMu.RLock()
	store := emailNotificationStore
	emailNotificationStoreMu.RUnlock()
	if store == nil {
		return EmailNotification{}
	}
	setting, err := store.GetByKeyContext(ctx, emailNotificationSettingKey)
	if err != nil || setting == nil || setting.ValueJSON == nil {
		return EmailNotification{}
	}
	return emailNotificationFromSetting(setting.ValueJSON)
}

func emailNotificationFromSetting(value map[string]any) EmailNotification {
	policy := EmailNotification{}
	if enabled, ok := boolSetting(value["enabled"]); ok {
		policy.Enabled = enabled
	}
	if host, ok := stringSetting(value["smtp_host"]); ok {
		policy.SMTPHost = host
	}
	if port, ok := intSettingPort(value["smtp_port"]); ok {
		policy.SMTPPort = port
	}
	if user, ok := stringSetting(value["username"]); ok {
		policy.Username = user
	}
	if pass, ok := stringSetting(value["password"]); ok {
		policy.Password = pass
	}
	if sender, ok := stringSetting(value["sender"]); ok {
		policy.Sender = sender
	}
	if useTLS, ok := boolSetting(value["use_tls"]); ok {
		policy.UseTLS = useTLS
	}
	if startTLS, ok := boolSetting(value["start_tls"]); ok {
		policy.StartTLS = startTLS
	}
	// Fail closed: enable only when a host is actually configured.
	policy.Enabled = policy.Enabled && policy.SMTPHost != ""
	return policy
}

func boolSetting(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}

func stringSetting(value any) (string, bool) {
	if s, ok := value.(string); ok {
		return s, true
	}
	return "", false
}

func intSettingPort(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}
