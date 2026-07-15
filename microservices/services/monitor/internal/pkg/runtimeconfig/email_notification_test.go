package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/gorm"
)

func TestEmailNotificationReaderFallsBackToStaticConfig(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:         true,
		SMTPHost:        "smtp.static.example.com",
		SMTPPort:        2525,
		Username:        "smtp-user",
		Password:        "smtp-password",
		Sender:          "admin@example.com",
		AlertReceivers:  []string{"ops@example.com"},
		SubjectTemplate: "Static {{title}}",
		BodyTemplate:    "Body {{content}}",
		RecipientGroups: map[string][]string{
			"notice": {"notice@example.com"},
		},
		UseTLS: true,
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{err: gorm.ErrRecordNotFound}, time.Minute)
	policy := reader.EmailNotification(context.Background())

	if !policy.Enabled {
		t.Fatal("email notification should use static enabled value")
	}
	if policy.SMTPHost != "smtp.static.example.com" || policy.SMTPPort != 2525 {
		t.Fatalf("smtp endpoint = %s:%d, want static config", policy.SMTPHost, policy.SMTPPort)
	}
	if policy.Username != "smtp-user" || policy.Password != "smtp-password" {
		t.Fatalf("smtp credentials = %q/%q, want static config credentials", policy.Username, policy.Password)
	}
	if policy.Sender != "admin@example.com" || len(policy.AlertReceivers) != 1 || policy.AlertReceivers[0] != "ops@example.com" {
		t.Fatalf("sender/receivers = %q/%#v, want static config values", policy.Sender, policy.AlertReceivers)
	}
	if policy.SubjectTemplate != "Static {{title}}" || policy.BodyTemplate != "Body {{content}}" {
		t.Fatalf("templates = %q/%q, want static config templates", policy.SubjectTemplate, policy.BodyTemplate)
	}
	if got := policy.RecipientGroups["notice"]; len(got) != 1 || got[0] != "notice@example.com" {
		t.Fatalf("recipient group notice = %#v, want static config group", got)
	}
	if !policy.UseTLS || policy.StartTLS {
		t.Fatalf("tls modes = use_tls:%v start_tls:%v, want static use_tls only", policy.UseTLS, policy.StartTLS)
	}
}

func TestEmailNotificationReaderAppliesNonSecretSettingOverrides(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:        false,
		SMTPHost:       "smtp.static.example.com",
		SMTPPort:       587,
		Username:       "smtp-user",
		Password:       "smtp-password",
		Sender:         "admin@example.com",
		AlertReceivers: []string{"static-ops@example.com"},
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"enabled":        true,
			"smtp_host":      "smtp.runtime.example.com",
			"sender":         "runtime@example.com",
			"alert_receiver": "ops@example.com, dev@example.com",
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if !policy.Enabled {
		t.Fatal("enabled should be overridden by runtime setting")
	}
	if policy.SMTPHost != "smtp.runtime.example.com" {
		t.Fatalf("smtp host = %q, want runtime override", policy.SMTPHost)
	}
	if policy.SMTPPort != 587 || policy.Username != "smtp-user" || policy.Password != "smtp-password" {
		t.Fatalf("secret/static values = %#v, want YAML-only values preserved", policy)
	}
	if policy.Sender != "runtime@example.com" {
		t.Fatalf("sender = %q, want runtime override", policy.Sender)
	}
	if got := policy.AlertReceivers; len(got) != 2 || got[0] != "ops@example.com" || got[1] != "dev@example.com" {
		t.Fatalf("alert receivers = %#v, want split runtime override", got)
	}
}

func TestEmailNotificationReaderAppliesRuntimeTemplatesAndRecipientGroups(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:         true,
		SMTPHost:        "smtp.static.example.com",
		SMTPPort:        587,
		Username:        "smtp-user",
		Password:        "smtp-password",
		Sender:          "admin@example.com",
		AlertReceivers:  []string{"static-ops@example.com"},
		SubjectTemplate: "Static {{title}}",
		BodyTemplate:    "Static {{content}}",
		RecipientGroups: map[string][]string{
			"notice": {"static-notice@example.com"},
		},
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"subject_template": "Runtime {{id}} {{title}}",
			"body_template":    "Runtime {{type}} {{content}}",
			"recipient_groups": map[string]any{
				"notice": "ops@example.com, dev@example.com",
				"audit":  []any{"audit@example.com"},
			},
			"username": "runtime-user",
			"password": "runtime-password",
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if policy.SubjectTemplate != "Runtime {{id}} {{title}}" {
		t.Fatalf("subject template = %q, want runtime override", policy.SubjectTemplate)
	}
	if policy.BodyTemplate != "Runtime {{type}} {{content}}" {
		t.Fatalf("body template = %q, want runtime override", policy.BodyTemplate)
	}
	if got := policy.RecipientGroups["notice"]; len(got) != 2 || got[0] != "ops@example.com" || got[1] != "dev@example.com" {
		t.Fatalf("recipient group notice = %#v, want split runtime recipients", got)
	}
	if got := policy.RecipientGroups["audit"]; len(got) != 1 || got[0] != "audit@example.com" {
		t.Fatalf("recipient group audit = %#v, want runtime list recipients", got)
	}
	if policy.Username != "smtp-user" || policy.Password != "smtp-password" {
		t.Fatalf("credentials = %q/%q, want runtime credentials ignored", policy.Username, policy.Password)
	}
}

func TestEmailNotificationReaderCanClearRuntimeTemplatesAndRecipientGroups(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:         true,
		SMTPHost:        "smtp.static.example.com",
		SMTPPort:        587,
		Username:        "smtp-user",
		Password:        "smtp-password",
		Sender:          "admin@example.com",
		AlertReceivers:  []string{"static-ops@example.com"},
		SubjectTemplate: "Static {{title}}",
		BodyTemplate:    "Static {{content}}",
		RecipientGroups: map[string][]string{
			"notice": {"static-notice@example.com"},
		},
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"subject_template": "",
			"body_template":    "   ",
			"recipient_groups": map[string]any{
				"notice": []any{},
			},
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if policy.SubjectTemplate != "" {
		t.Fatalf("subject template = %q, want cleared runtime override", policy.SubjectTemplate)
	}
	if policy.BodyTemplate != "" {
		t.Fatalf("body template = %q, want cleared runtime override", policy.BodyTemplate)
	}
	if len(policy.RecipientGroups) != 0 {
		t.Fatalf("recipient groups = %#v, want cleared runtime override", policy.RecipientGroups)
	}
}

func TestEmailNotificationReaderAppliesTLSModeOverrides(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:        true,
		SMTPHost:       "smtp.static.example.com",
		SMTPPort:       587,
		Username:       "smtp-user",
		Password:       "smtp-password",
		Sender:         "admin@example.com",
		AlertReceivers: []string{"static-ops@example.com"},
		UseTLS:         true,
		StartTLS:       false,
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"use_tls":   false,
			"start_tls": true,
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if policy.UseTLS {
		t.Fatal("use_tls should be overridden to false by runtime setting")
	}
	if !policy.StartTLS {
		t.Fatal("start_tls should be overridden to true by runtime setting")
	}
	if policy.Username != "smtp-user" || policy.Password != "smtp-password" {
		t.Fatalf("credentials = %q/%q, want YAML-only values preserved", policy.Username, policy.Password)
	}
}

func TestEmailNotificationReaderFallsBackToStaticConfigWhenRuntimeTLSModesConflict(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:        true,
		SMTPHost:       "smtp.static.example.com",
		SMTPPort:       587,
		Username:       "smtp-user",
		Password:       "smtp-password",
		Sender:         "admin@example.com",
		AlertReceivers: []string{"static-ops@example.com"},
		UseTLS:         true,
		StartTLS:       false,
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"use_tls":   true,
			"start_tls": true,
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if !policy.UseTLS || policy.StartTLS {
		t.Fatalf("tls modes = use_tls:%v start_tls:%v, want static config after conflicting runtime setting", policy.UseTLS, policy.StartTLS)
	}
}

func TestEmailNotificationReaderAllowsRuntimeSettingToDisableStaticEmail(t *testing.T) {
	oldNotification := config.Cfg.Notification
	config.Cfg.Notification.Email = config.EmailConfig{
		Enabled:        true,
		SMTPHost:       "smtp.static.example.com",
		SMTPPort:       2525,
		Username:       "smtp-user",
		Password:       "smtp-password",
		Sender:         "admin@example.com",
		AlertReceivers: []string{"ops@example.com"},
		UseTLS:         true,
	}
	t.Cleanup(func() {
		config.Cfg.Notification = oldNotification
	})

	reader := NewCachedEmailNotificationReader(&stubEmailNotificationStore{setting: &model.SystemSetting{
		SettingKey: EmailNotificationSettingKey,
		ValueJSON: map[string]any{
			"enabled":        false,
			"smtp_host":      "",
			"sender":         "",
			"alert_receiver": "",
		},
	}}, time.Minute)

	policy := reader.EmailNotification(context.Background())

	if policy.Enabled {
		t.Fatal("enabled:false runtime setting should disable static email notification")
	}
	if policy.SMTPHost != "smtp.static.example.com" || policy.Sender != "admin@example.com" {
		t.Fatalf("static sender config was overwritten: %#v", policy)
	}
	if got := policy.AlertReceivers; len(got) != 1 || got[0] != "ops@example.com" {
		t.Fatalf("alert receivers = %#v, want static recipients preserved", got)
	}
	if !policy.UseTLS || policy.StartTLS {
		t.Fatalf("tls modes = use_tls:%v start_tls:%v, want static TLS config preserved", policy.UseTLS, policy.StartTLS)
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
		return nil, errors.New("missing setting")
	}
	return s.setting, nil
}
