package system

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/mailer"
	"github.com/go-admin-kit/server/internal/pkg/runtimeconfig"
)

func TestNoticeEmailNotifierSendsEnabledNoticeEmail(t *testing.T) {
	sender := &stubEmailSender{}
	notifier := NewNoticeEmailNotifier(sender, stubEmailNotificationReader{policy: runtimeconfig.EmailNotification{
		Enabled:        true,
		Sender:         "admin@example.com",
		AlertReceivers: []string{"ops@example.com", "dev@example.com"},
	}})

	err := notifier.SendNoticeEnabledContext(context.Background(), &model.Notice{
		ID:      7,
		Title:   "Maintenance",
		Content: "Maintenance window tonight",
		Type:    2,
		Status:  1,
	})
	if err != nil {
		t.Fatalf("SendNoticeEnabledContext() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.messages))
	}

	message := sender.messages[0]
	if message.From != "admin@example.com" {
		t.Fatalf("from = %q, want admin@example.com", message.From)
	}
	if strings.Join(message.To, ",") != "ops@example.com,dev@example.com" {
		t.Fatalf("to = %#v, want configured alert receivers", message.To)
	}
	if message.Subject != "Notice enabled: Maintenance" {
		t.Fatalf("subject = %q, want notice subject", message.Subject)
	}
	for _, want := range []string{
		"Notice ID: 7",
		"Type: announcement",
		"Title: Maintenance",
		"Content:",
		"Maintenance window tonight",
	} {
		if !strings.Contains(message.Body, want) {
			t.Fatalf("body %q does not contain %q", message.Body, want)
		}
	}
}

func TestNoticeEmailNotifierUsesNoticeGroupAndTemplates(t *testing.T) {
	sender := &stubEmailSender{}
	notifier := NewNoticeEmailNotifier(sender, stubEmailNotificationReader{policy: runtimeconfig.EmailNotification{
		Enabled:         true,
		Sender:          "admin@example.com",
		AlertReceivers:  []string{"fallback@example.com"},
		SubjectTemplate: "Notice {{id}} {{type}} {{title}} {{status}}",
		BodyTemplate:    "{{title}}\n{{content}}\n{{missing}}",
		RecipientGroups: map[string][]string{
			"notice": {"ops@example.com", "dev@example.com"},
		},
	}})

	err := notifier.SendNoticeEnabledContext(context.Background(), &model.Notice{
		ID:      9,
		Title:   "Deploy",
		Content: "Deploy finished",
		Type:    1,
		Status:  1,
	})
	if err != nil {
		t.Fatalf("SendNoticeEnabledContext() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.messages))
	}

	message := sender.messages[0]
	if strings.Join(message.To, ",") != "ops@example.com,dev@example.com" {
		t.Fatalf("to = %#v, want notice recipient group", message.To)
	}
	if message.Subject != "Notice 9 notice Deploy {{status}}" {
		t.Fatalf("subject = %q, want safe placeholder replacement only", message.Subject)
	}
	if message.Body != "Deploy\nDeploy finished\n{{missing}}" {
		t.Fatalf("body = %q, want templated body with unknown placeholder preserved", message.Body)
	}
}

func TestNoticeEmailNotifierFallsBackToAlertReceiversWhenNoticeGroupMissing(t *testing.T) {
	sender := &stubEmailSender{}
	notifier := NewNoticeEmailNotifier(sender, stubEmailNotificationReader{policy: runtimeconfig.EmailNotification{
		Enabled:        true,
		Sender:         "admin@example.com",
		AlertReceivers: []string{"fallback@example.com"},
		RecipientGroups: map[string][]string{
			"audit": {"audit@example.com"},
		},
	}})

	err := notifier.SendNoticeEnabledContext(context.Background(), &model.Notice{
		Title:  "Maintenance",
		Status: 1,
	})
	if err != nil {
		t.Fatalf("SendNoticeEnabledContext() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.messages))
	}
	if strings.Join(sender.messages[0].To, ",") != "fallback@example.com" {
		t.Fatalf("to = %#v, want alert receivers fallback", sender.messages[0].To)
	}
}

func TestNoticeEmailNotifierSkipsDisabledPolicy(t *testing.T) {
	sender := &stubEmailSender{}
	notifier := NewNoticeEmailNotifier(sender, stubEmailNotificationReader{policy: runtimeconfig.EmailNotification{
		Enabled:        false,
		Sender:         "admin@example.com",
		AlertReceivers: []string{"ops@example.com"},
	}})

	err := notifier.SendNoticeEnabledContext(context.Background(), &model.Notice{
		Title:  "Maintenance",
		Status: 1,
	})
	if err != nil {
		t.Fatalf("SendNoticeEnabledContext() error = %v, want nil", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("sent messages = %d, want 0", len(sender.messages))
	}
}

func TestNoticeEmailNotifierReturnsSenderError(t *testing.T) {
	sendErr := errors.New("smtp unavailable")
	sender := &stubEmailSender{err: sendErr}
	notifier := NewNoticeEmailNotifier(sender, stubEmailNotificationReader{policy: runtimeconfig.EmailNotification{
		Enabled:        true,
		Sender:         "admin@example.com",
		AlertReceivers: []string{"ops@example.com"},
	}})

	err := notifier.SendNoticeEnabledContext(context.Background(), &model.Notice{
		Title:  "Maintenance",
		Status: 1,
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("SendNoticeEnabledContext() error = %v, want sender error", err)
	}
}

type stubEmailSender struct {
	messages []mailer.Message
	err      error
}

func (s *stubEmailSender) Send(ctx context.Context, message mailer.Message) error {
	s.messages = append(s.messages, message)
	return s.err
}

type stubEmailNotificationReader struct {
	policy runtimeconfig.EmailNotification
}

func (s stubEmailNotificationReader) EmailNotification(ctx context.Context) runtimeconfig.EmailNotification {
	return s.policy
}
