package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/mailer"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
)

type AlertEmailNotifier struct {
	sender mailer.Sender
	reader runtimeconfig.EmailNotificationReader
	now    func() time.Time
}

func NewAlertEmailNotifier(sender mailer.Sender, reader runtimeconfig.EmailNotificationReader) *AlertEmailNotifier {
	return &AlertEmailNotifier{sender: sender, reader: reader, now: time.Now}
}

func DefaultAlertEmailNotifier() *AlertEmailNotifier {
	return NewAlertEmailNotifier(nil, runtimeconfig.DefaultEmailNotificationReader())
}

func (n *AlertEmailNotifier) NotifyContext(ctx context.Context, _ *model.MonitorAlertRule, event *model.MonitorAlertEvent) AlertNotification {
	now := time.Now
	if n != nil && n.now != nil {
		now = n.now
	}
	result := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: now().UTC()}
	if event == nil {
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var reader runtimeconfig.EmailNotificationReader
	var sender mailer.Sender
	if n != nil {
		reader = n.reader
		sender = n.sender
	}
	if reader == nil {
		reader = runtimeconfig.DefaultEmailNotificationReader()
	}
	policy := reader.EmailNotification(ctx)
	recipients := alertEmailRecipients(policy)
	if !policy.Enabled || len(recipients) == 0 {
		return result
	}
	if sender == nil {
		sender = mailer.NewSMTPSender(mailer.SMTPConfig{
			Enabled:  policy.Enabled,
			SMTPHost: policy.SMTPHost,
			SMTPPort: policy.SMTPPort,
			Username: policy.Username,
			Password: policy.Password,
			Sender:   policy.Sender,
			UseTLS:   policy.UseTLS,
			StartTLS: policy.StartTLS,
		}, nil)
	}
	err := sender.Send(ctx, mailer.Message{
		From:    policy.Sender,
		To:      recipients,
		Subject: alertEmailSubject(policy, event),
		Body:    alertEmailBody(policy, event),
	})
	if err != nil {
		result.Status = AlertNotifyFailed
		result.Error = err.Error()
		return result
	}
	result.Status = AlertNotifySent
	return result
}

func alertEmailRecipients(policy runtimeconfig.EmailNotification) []string {
	if recipients := policy.RecipientGroups["alert"]; len(recipients) > 0 {
		return recipients
	}
	return policy.AlertReceivers
}

func alertEmailSubject(policy runtimeconfig.EmailNotification, event *model.MonitorAlertEvent) string {
	if strings.TrimSpace(policy.SubjectTemplate) != "" {
		return renderAlertEmailTemplate(policy.SubjectTemplate, event)
	}
	return fmt.Sprintf("[%s] Alert %s: %s", strings.ToUpper(event.Severity), event.Status, event.RuleName)
}

func alertEmailBody(policy runtimeconfig.EmailNotification, event *model.MonitorAlertEvent) string {
	if strings.TrimSpace(policy.BodyTemplate) != "" {
		return renderAlertEmailTemplate(policy.BodyTemplate, event)
	}
	return fmt.Sprintf("Rule: %s\nStatus: %s\nSeverity: %s\nMetric: %s\nValue: %.4f\nThreshold: %.4f\nMessage: %s\nTime: %s\n",
		event.RuleName,
		event.Status,
		event.Severity,
		event.Metric,
		event.Value,
		event.Threshold,
		event.Message,
		event.CreatedAt.UTC().Format(time.RFC3339),
	)
}

func renderAlertEmailTemplate(template string, event *model.MonitorAlertEvent) string {
	return strings.NewReplacer(
		"{{id}}", fmt.Sprint(event.ID),
		"{{type}}", "alert",
		"{{title}}", event.RuleName,
		"{{content}}", event.Message,
		"{{rule_name}}", event.RuleName,
		"{{status}}", event.Status,
		"{{severity}}", event.Severity,
		"{{metric}}", event.Metric,
		"{{value}}", fmt.Sprintf("%.4f", event.Value),
		"{{threshold}}", fmt.Sprintf("%.4f", event.Threshold),
	).Replace(template)
}
