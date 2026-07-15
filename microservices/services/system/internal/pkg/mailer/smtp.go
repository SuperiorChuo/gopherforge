package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidConfig = errors.New("invalid email config")

const defaultSMTPTimeout = 10 * time.Second

type Message struct {
	From    string
	To      []string
	Subject string
	Body    string
}

type SMTPConfig struct {
	Enabled  bool
	SMTPHost string
	SMTPPort int
	Username string
	Password string
	Sender   string
	UseTLS   bool
	StartTLS bool
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}

type TransportFunc func(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error

type SMTPSender struct {
	config    SMTPConfig
	transport TransportFunc
}

func NewSMTPSender(config SMTPConfig, transport TransportFunc) *SMTPSender {
	return &SMTPSender{config: config, transport: transport}
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || !s.config.Enabled {
		return nil
	}

	recipients := cleanRecipients(message.To)
	if len(recipients) == 0 {
		return nil
	}

	from := strings.TrimSpace(message.From)
	if from == "" {
		from = strings.TrimSpace(s.config.Sender)
	}
	if strings.TrimSpace(s.config.SMTPHost) == "" {
		return fmt.Errorf("%w: smtp host is required", ErrInvalidConfig)
	}
	if from == "" {
		return fmt.Errorf("%w: sender is required", ErrInvalidConfig)
	}
	if s.config.UseTLS && s.config.StartTLS {
		return fmt.Errorf("%w: use_tls and start_tls cannot both be true", ErrInvalidConfig)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	addr := smtpAddress(s.config)
	auth := smtpAuth(s.config)
	msg := buildMessage(from, recipients, message.Subject, message.Body)
	if s.transport != nil {
		return s.transport(ctx, addr, auth, from, recipients, msg)
	}
	return defaultTransport(ctx, s.config, auth, from, recipients, msg)
}

func defaultTransport(ctx context.Context, config SMTPConfig, auth smtp.Auth, from string, to []string, msg []byte) error {
	return sendSMTPWithDeps(ctx, config, auth, from, to, msg, defaultSMTPTransportDeps())
}

type smtpClient interface {
	Extension(ext string) (bool, string)
	StartTLS(config *tls.Config) error
	Auth(auth smtp.Auth) error
	Mail(from string) error
	Rcpt(to string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpTransportDeps struct {
	dialContext    func(ctx context.Context, network string, addr string) (net.Conn, error)
	tlsDialContext func(ctx context.Context, network string, addr string, config *tls.Config) (net.Conn, error)
	newClient      func(conn net.Conn, host string) (smtpClient, error)
	now            func() time.Time
}

func defaultSMTPTransportDeps() smtpTransportDeps {
	dialer := &net.Dialer{Timeout: defaultSMTPTimeout}
	return smtpTransportDeps{
		dialContext: dialer.DialContext,
		tlsDialContext: func(ctx context.Context, network string, addr string, config *tls.Config) (net.Conn, error) {
			tlsDialer := tls.Dialer{NetDialer: dialer, Config: config}
			return tlsDialer.DialContext(ctx, network, addr)
		},
		newClient: func(conn net.Conn, host string) (smtpClient, error) {
			return smtp.NewClient(conn, host)
		},
		now: time.Now,
	}
}

func sendSMTPWithDeps(ctx context.Context, config SMTPConfig, auth smtp.Auth, from string, to []string, msg []byte, deps smtpTransportDeps) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if config.UseTLS && config.StartTLS {
		return fmt.Errorf("%w: use_tls and start_tls cannot both be true", ErrInvalidConfig)
	}

	deps = deps.withDefaults()
	addr := smtpAddress(config)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	tlsConfig := &tls.Config{ServerName: host}

	var conn net.Conn
	if config.UseTLS {
		conn, err = deps.tlsDialContext(ctx, "tcp", addr, tlsConfig)
	} else {
		conn, err = deps.dialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := deps.now().Add(defaultSMTPTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	client, err := deps.newClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if config.StartTLS {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("%w: smtp server does not support AUTH", ErrInvalidConfig)
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (d smtpTransportDeps) withDefaults() smtpTransportDeps {
	defaults := defaultSMTPTransportDeps()
	if d.dialContext == nil {
		d.dialContext = defaults.dialContext
	}
	if d.tlsDialContext == nil {
		d.tlsDialContext = defaults.tlsDialContext
	}
	if d.newClient == nil {
		d.newClient = defaults.newClient
	}
	if d.now == nil {
		d.now = defaults.now
	}
	return d
}

func cleanRecipients(values []string) []string {
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			recipients = append(recipients, value)
		}
	}
	return recipients
}

func smtpAddress(config SMTPConfig) string {
	host := strings.TrimSpace(config.SMTPHost)
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	port := config.SMTPPort
	if port <= 0 {
		port = 25
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func smtpAuth(config SMTPConfig) smtp.Auth {
	username := strings.TrimSpace(config.Username)
	if username == "" && config.Password == "" {
		return nil
	}
	host := strings.TrimSpace(config.SMTPHost)
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	return smtp.PlainAuth("", username, config.Password, host)
}

func buildMessage(from string, to []string, subject string, body string) []byte {
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(strings.Join(to, ", ")),
		"Subject: " + sanitizeHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
