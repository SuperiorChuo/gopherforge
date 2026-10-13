package channel

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPConfig for outbound mail. Empty Host disables the sender.
type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string // "Name <addr@x.com>" or bare address
	// TLS: when true use STARTTLS on submission ports if available; plain for local.
	// M2 uses net/smtp SendMail which upgrades when server supports it via auth.
}

// SMTPEmail is a minimal SMTP adapter (stdlib net/smtp).
type SMTPEmail struct {
	cfg SMTPConfig
}

func NewSMTPEmail(cfg SMTPConfig) *SMTPEmail {
	return &SMTPEmail{cfg: cfg}
}

// NewDynamicSMTPEmail 每次探活/发送实时解析配置（DB 热配置，M3 邮件管理面）：
// 控制台改 SMTP 账号后热生效，无需重启进程。
func NewDynamicSMTPEmail(resolve func(context.Context) SMTPConfig) EmailSender {
	return &dynamicSMTPEmail{resolve: resolve}
}

type dynamicSMTPEmail struct {
	resolve func(context.Context) SMTPConfig
}

func (d *dynamicSMTPEmail) Name() string { return "smtp" }

func (d *dynamicSMTPEmail) Enabled() bool {
	return NewSMTPEmail(d.resolve(context.Background())).Enabled()
}

func (d *dynamicSMTPEmail) Send(ctx context.Context, to string, p Payload) error {
	return NewSMTPEmail(d.resolve(ctx)).Send(ctx, to, p)
}

func (s *SMTPEmail) Name() string { return "smtp" }

func (s *SMTPEmail) Enabled() bool {
	return s != nil && strings.TrimSpace(s.cfg.Host) != "" && strings.TrimSpace(s.cfg.From) != ""
}

func (s *SMTPEmail) Send(ctx context.Context, to string, p Payload) error {
	if !s.Enabled() {
		return fmt.Errorf("smtp disabled")
	}
	to = strings.TrimSpace(to)
	if to == "" || !strings.Contains(to, "@") {
		return fmt.Errorf("invalid email")
	}
	from := strings.TrimSpace(s.cfg.From)
	// extract bare address for MAIL FROM if "Name <a@b>"
	fromAddr := from
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.LastIndex(from, ">"); j > i {
			fromAddr = strings.TrimSpace(from[i+1 : j])
		}
	}
	port := s.cfg.Port
	if port == "" {
		port = "587"
	}
	addr := net.JoinHostPort(s.cfg.Host, port)

	subject := p.Title
	body := p.Content
	if p.Link != "" {
		body = body + "\n\n链接: " + p.Link
	}
	msg := buildMIME(from, to, subject, body)

	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)
	}

	// honor context deadline via short dial if canceled
	if err := ctx.Err(); err != nil {
		return err
	}
	// net/smtp has no context; use goroutine + select on timeout
	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		ch <- result{err: smtp.SendMail(addr, auth, fromAddr, []string{to}, msg)}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-ch:
		return r.err
	case <-time.After(20 * time.Second):
		return fmt.Errorf("smtp timeout")
	}
}

func buildMIME(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeSubject(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// encodeSubject keeps ASCII subjects plain; non-ASCII use RFC 2047 UTF-8 B encoding.
func encodeSubject(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}
