package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/mailer"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
)

type NoticeEmailNotifier struct {
	sender mailer.Sender
	reader runtimeconfig.EmailNotificationReader
}

func NewNoticeEmailNotifier(sender mailer.Sender, reader runtimeconfig.EmailNotificationReader) *NoticeEmailNotifier {
	return &NoticeEmailNotifier{sender: sender, reader: reader}
}

func DefaultNoticeEmailNotifier() *NoticeEmailNotifier {
	return NewNoticeEmailNotifier(nil, runtimeconfig.DefaultEmailNotificationReader())
}

func (n *NoticeEmailNotifier) SendNoticeEnabledContext(ctx context.Context, notice *model.Notice) error {
	if notice == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reader := n.reader
	if reader == nil {
		reader = runtimeconfig.DefaultEmailNotificationReader()
	}
	policy := reader.EmailNotification(ctx)
	recipients := noticeEmailRecipients(policy)
	if !policy.Enabled || len(recipients) == 0 {
		return nil
	}

	sender := n.sender
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
	return sender.Send(ctx, mailer.Message{
		From:    policy.Sender,
		To:      recipients,
		Subject: noticeEmailSubject(policy, notice),
		Body:    noticeEmailBody(policy, notice),
	})
}

func noticeEmailRecipients(policy runtimeconfig.EmailNotification) []string {
	if recipients := policy.RecipientGroups["notice"]; len(recipients) > 0 {
		return recipients
	}
	return policy.AlertReceivers
}

func noticeEmailSubject(policy runtimeconfig.EmailNotification, notice *model.Notice) string {
	if strings.TrimSpace(policy.SubjectTemplate) != "" {
		return renderNoticeEmailTemplate(policy.SubjectTemplate, notice)
	}
	title := strings.TrimSpace(notice.Title)
	if title == "" {
		title = "Untitled"
	}
	return "Notice enabled: " + title
}

func noticeEmailBody(policy runtimeconfig.EmailNotification, notice *model.Notice) string {
	if strings.TrimSpace(policy.BodyTemplate) != "" {
		return renderNoticeEmailTemplate(policy.BodyTemplate, notice)
	}
	return fmt.Sprintf("Notice ID: %d\nType: %s\nTitle: %s\nContent:\n%s\n",
		notice.ID,
		noticeEmailType(notice.Type),
		notice.Title,
		notice.Content,
	)
}

func renderNoticeEmailTemplate(template string, notice *model.Notice) string {
	replacer := strings.NewReplacer(
		"{{id}}", fmt.Sprint(notice.ID),
		"{{type}}", noticeEmailType(notice.Type),
		"{{title}}", notice.Title,
		"{{content}}", notice.Content,
	)
	return replacer.Replace(template)
}

func noticeEmailType(noticeType int8) string {
	if noticeType == 2 {
		return "announcement"
	}
	return "notice"
}
