package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/system/internal/config"
	"github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

const EmailNotificationSettingKey = "notification.email"

type EmailNotification struct {
	Enabled         bool
	SMTPHost        string
	SMTPPort        int
	Username        string
	Password        string
	Sender          string
	AlertReceivers  []string
	SubjectTemplate string
	BodyTemplate    string
	RecipientGroups map[string][]string
	UseTLS          bool
	StartTLS        bool
}

type EmailNotificationReader interface {
	EmailNotification(ctx context.Context) EmailNotification
}

type EmailNotificationInvalidator interface {
	Refresh(ctx context.Context) error
}

type EmailNotificationStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

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
	defaultEmailNotificationOnce   sync.Once
	defaultEmailNotificationReader *CachedEmailNotificationReader
)

func DefaultEmailNotificationReader() *CachedEmailNotificationReader {
	defaultEmailNotificationOnce.Do(func() {
		defaultEmailNotificationReader = NewCachedEmailNotificationReader(defaultSecurityPolicyStore{}, 30*time.Second)
	})
	return defaultEmailNotificationReader
}

func (r *CachedEmailNotificationReader) EmailNotification(ctx context.Context) EmailNotification {
	if r == nil {
		return EmailNotificationFromConfig()
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		policy := r.policy
		r.mu.RUnlock()
		return policy
	}
	r.mu.RUnlock()

	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		if r.loaded {
			policy := r.policy
			r.mu.RUnlock()
			return policy
		}
		r.mu.RUnlock()
		return EmailNotificationFromConfig()
	}

	r.mu.RLock()
	policy := r.policy
	r.mu.RUnlock()
	return policy
}

func (r *CachedEmailNotificationReader) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	policy := EmailNotificationFromConfig()
	var err error
	if r.store != nil {
		var setting *model.SystemSetting
		setting, err = r.store.GetByKeyContext(ctx, EmailNotificationSettingKey)
		switch {
		case err == nil && setting != nil:
			policy = applyEmailNotificationSetting(policy, setting.ValueJSON)
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = nil
		}
	}

	if err == nil {
		r.mu.Lock()
		r.policy = policy
		r.expiresAt = time.Now().Add(r.ttl)
		r.loaded = true
		r.mu.Unlock()
	}
	return err
}

func EmailNotificationFromConfig() EmailNotification {
	email := config.Cfg.Notification.Email
	return EmailNotification{
		Enabled:         email.Enabled,
		SMTPHost:        strings.TrimSpace(email.SMTPHost),
		SMTPPort:        positiveOrDefault(email.SMTPPort, 25),
		Username:        strings.TrimSpace(email.Username),
		Password:        email.Password,
		Sender:          strings.TrimSpace(email.Sender),
		AlertReceivers:  configuredRecipients(email.AlertReceivers, email.AlertReceiver),
		SubjectTemplate: email.SubjectTemplate,
		BodyTemplate:    email.BodyTemplate,
		RecipientGroups: configuredRecipientGroups(email.RecipientGroups),
		UseTLS:          email.UseTLS,
		StartTLS:        email.StartTLS,
	}
}

func applyEmailNotificationSetting(policy EmailNotification, value map[string]any) EmailNotification {
	if value == nil {
		return policy
	}
	staticPolicy := policy
	if !hasEmailNotificationOverride(value) {
		return policy
	}
	if enabled, ok := boolSetting(value["enabled"]); ok {
		policy.Enabled = enabled
	}
	if smtpHost, ok := stringSetting(value["smtp_host"]); ok {
		policy.SMTPHost = smtpHost
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
	if recipients, ok := recipientsSetting(value["alert_receiver"]); ok {
		policy.AlertReceivers = recipients
	} else if recipients, ok := recipientsSetting(value["alert_receivers"]); ok {
		policy.AlertReceivers = recipients
	}
	if rawSubjectTemplate, exists := value["subject_template"]; exists {
		if subjectTemplate, ok := templateSetting(rawSubjectTemplate); ok {
			policy.SubjectTemplate = subjectTemplate
		}
	}
	if rawBodyTemplate, exists := value["body_template"]; exists {
		if bodyTemplate, ok := templateSetting(rawBodyTemplate); ok {
			policy.BodyTemplate = bodyTemplate
		}
	}
	if rawRecipientGroups, exists := value["recipient_groups"]; exists {
		if recipientGroups, ok := recipientGroupsSetting(rawRecipientGroups); ok {
			policy.RecipientGroups = recipientGroups
		}
	}
	if policy.UseTLS && policy.StartTLS {
		return staticPolicy
	}
	return policy
}

func hasEmailNotificationOverride(value map[string]any) bool {
	if _, ok := boolSetting(value["enabled"]); ok {
		return true
	}
	for _, key := range []string{"use_tls", "start_tls"} {
		if _, ok := boolSetting(value[key]); ok {
			return true
		}
	}
	for _, key := range []string{"smtp_host", "sender"} {
		if _, ok := stringSetting(value[key]); ok {
			return true
		}
	}
	for _, key := range []string{"subject_template", "body_template"} {
		rawValue, exists := value[key]
		if !exists {
			continue
		}
		if _, ok := templateSetting(rawValue); ok {
			return true
		}
	}
	for _, key := range []string{"alert_receiver", "alert_receivers"} {
		if _, ok := recipientsSetting(value[key]); ok {
			return true
		}
	}
	if rawRecipientGroups, exists := value["recipient_groups"]; exists {
		if _, ok := recipientGroupsSetting(rawRecipientGroups); ok {
			return true
		}
	}
	return false
}

func boolSetting(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed := strings.TrimSpace(strings.ToLower(v))
		switch parsed {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func stringSetting(value any) (string, bool) {
	v, ok := value.(string)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

func templateSetting(value any) (string, bool) {
	v, ok := value.(string)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(v) == "" {
		return "", true
	}
	return v, true
}

func recipientsSetting(value any) ([]string, bool) {
	switch v := value.(type) {
	case string:
		recipients := splitRecipients(v)
		return recipients, len(recipients) > 0
	case []string:
		recipients := configuredRecipients(v, "")
		return recipients, len(recipients) > 0
	case []any:
		recipients := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				recipients = append(recipients, str)
			}
		}
		recipients = configuredRecipients(recipients, "")
		return recipients, len(recipients) > 0
	default:
		return nil, false
	}
}

func recipientGroupsSetting(value any) (map[string][]string, bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case map[string][]string:
		groups := configuredRecipientGroups(v)
		return groups, true
	case map[string]any:
		groups := make(map[string][]string, len(v))
		for key, rawRecipients := range v {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			recipients, ok := recipientsSetting(rawRecipients)
			if ok {
				groups[key] = recipients
			}
		}
		if len(groups) == 0 {
			return nil, true
		}
		return groups, true
	default:
		return nil, false
	}
}

func configuredRecipients(values []string, fallback string) []string {
	recipients := make([]string, 0, len(values)+1)
	if strings.TrimSpace(fallback) != "" {
		recipients = append(recipients, splitRecipients(fallback)...)
	}
	for _, value := range values {
		recipients = append(recipients, splitRecipients(value)...)
	}
	return recipients
}

func configuredRecipientGroups(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	groups := make(map[string][]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		recipients := configuredRecipients(value, "")
		if len(recipients) > 0 {
			groups[key] = recipients
		}
	}
	if len(groups) == 0 {
		return nil
	}
	return groups
}

func splitRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			recipients = append(recipients, part)
		}
	}
	return recipients
}
