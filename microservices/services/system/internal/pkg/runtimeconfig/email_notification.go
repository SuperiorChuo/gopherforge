package runtimeconfig

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/system/internal/config"
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
	reader      *sharedruntimeconfig.CachedSettingReader[EmailNotification]
	tenantStore TenantSettingStore
}

func NewCachedEmailNotificationReader(store EmailNotificationStore, ttl time.Duration) *CachedEmailNotificationReader {
	return &CachedEmailNotificationReader{
		reader: sharedruntimeconfig.NewCachedSettingReader(
			store, EmailNotificationSettingKey, ttl, EmailNotificationFromConfig,
			func(policy EmailNotification, value map[string]any) EmailNotification {
				return enforceEmailNotificationSafety(applyEmailNotificationSetting(policy, value))
			},
		),
		tenantStore: defaultTenantSettingStore{},
	}
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
	return r.applyTenantOverride(ctx, r.reader.Value(ctx))
}

// applyTenantOverride 显式租户上下文命中 tenant_settings 覆盖时应用之；
// 后台/无租户上下文维持平台默认。
func (r *CachedEmailNotificationReader) applyTenantOverride(ctx context.Context, policy EmailNotification) EmailNotification {
	if r == nil || r.tenantStore == nil {
		return policy
	}
	if override := tenantOverride(ctx, r.tenantStore, EmailNotificationSettingKey); override != nil {
		policy = applyEmailNotificationSetting(policy, override.ValueJSON)
	}
	return policy
}

func (r *CachedEmailNotificationReader) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.reader.Refresh(ctx)
}

func EmailNotificationFromConfig() EmailNotification {
	email := config.Cfg.Notification.Email
	return enforceEmailNotificationSafety(EmailNotification{
		Enabled:         email.Enabled,
		SMTPHost:        strings.TrimSpace(email.SMTPHost),
		SMTPPort:        sharedruntimeconfig.PositiveOrDefault(email.SMTPPort, 25),
		Username:        strings.TrimSpace(email.Username),
		Password:        email.Password,
		Sender:          strings.TrimSpace(email.Sender),
		AlertReceivers:  configuredRecipients(email.AlertReceivers, email.AlertReceiver),
		SubjectTemplate: email.SubjectTemplate,
		BodyTemplate:    email.BodyTemplate,
		RecipientGroups: configuredRecipientGroups(email.RecipientGroups),
		UseTLS:          email.UseTLS,
		StartTLS:        email.StartTLS,
	})
}

// enforceEmailNotificationSafety keeps the email channel closed on the one shape
// startup validation cannot see. config.Validate judged the environment derived
// settings, where EMAIL_NOTIFICATION_ENABLED is usually false, so a
// system_settings row that flips enabled on (and may supply its own smtp_host)
// can put a weak EMAIL_SMTP_PASSWORD on the wire at runtime in a process that
// was legitimately allowed to boot. Fail closed there: Enabled=false is what
// every sender checks before dialing SMTP, so an operator loses alert mail
// rather than leaking the password to whoever answers on port 25.
//
// The judgement is config.IsSMTPAuthUnsafe — literally the predicate the startup
// gate calls — so the two cannot drift. An anonymous relay (no username, no
// password) is not authentication and stays enabled; non-production is never
// touched.
func enforceEmailNotificationSafety(policy EmailNotification) EmailNotification {
	if !policy.Enabled {
		return policy
	}
	if !config.IsSMTPAuthUnsafe(config.Cfg.App.Env, config.EmailConfig{
		Enabled:  policy.Enabled,
		SMTPHost: policy.SMTPHost,
		Username: policy.Username,
		Password: policy.Password,
	}) {
		return policy
	}
	policy.Enabled = false
	warnEmailNotificationKeptDisabled(policy)
	return policy
}

// warnEmailNotificationKeptDisabled is a variable so tests can observe the
// warning without standing up the global logger. It must never log the
// credential itself.
var warnEmailNotificationKeptDisabled = func(policy EmailNotification) {
	if logger.Logger == nil {
		return
	}
	logger.Warn(
		"email notification kept disabled: this runtime setting would authenticate to SMTP with a weak or empty EMAIL_SMTP_PASSWORD, which production refuses",
		logger.String("setting_key", EmailNotificationSettingKey),
		logger.String("smtp_host", policy.SMTPHost),
		logger.String("resolution", "set a strong EMAIL_SMTP_PASSWORD in the service environment and restart, then enable the channel again"),
	)
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
