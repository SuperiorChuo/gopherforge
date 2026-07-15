package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/smtp"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSMTPSenderSendSkipsWhenDisabledOrWithoutRecipients(t *testing.T) {
	calls := 0
	transport := func(context.Context, string, smtp.Auth, string, []string, []byte) error {
		calls++
		return nil
	}

	disabled := NewSMTPSender(SMTPConfig{
		Enabled:  false,
		SMTPHost: "smtp.example.com",
		Sender:   "admin@example.com",
	}, transport)
	if err := disabled.Send(context.Background(), Message{
		To:      []string{"ops@example.com"},
		Subject: "Notice enabled",
		Body:    "Maintenance",
	}); err != nil {
		t.Fatalf("disabled sender Send() error = %v, want nil", err)
	}

	withoutRecipients := NewSMTPSender(SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		Sender:   "admin@example.com",
	}, transport)
	if err := withoutRecipients.Send(context.Background(), Message{
		Subject: "Notice enabled",
		Body:    "Maintenance",
	}); err != nil {
		t.Fatalf("sender without recipients Send() error = %v, want nil", err)
	}

	if calls != 0 {
		t.Fatalf("transport calls = %d, want 0", calls)
	}
}

func TestSMTPSenderSendBuildsEnvelopeAndMessage(t *testing.T) {
	var gotAddr string
	var gotFrom string
	var gotTo []string
	var gotBody string

	sender := NewSMTPSender(SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 2525,
		Username: "smtp-user",
		Password: "smtp-password",
		Sender:   "admin@example.com",
	}, func(_ context.Context, addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr = addr
		gotFrom = from
		gotTo = append([]string(nil), to...)
		gotBody = string(msg)
		return nil
	})

	err := sender.Send(context.Background(), Message{
		To:      []string{" ops@example.com ", "dev@example.com"},
		Subject: "Notice enabled",
		Body:    "Maintenance window tonight",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if gotAddr != "smtp.example.com:2525" {
		t.Fatalf("addr = %q, want smtp.example.com:2525", gotAddr)
	}
	if gotFrom != "admin@example.com" {
		t.Fatalf("from = %q, want admin@example.com", gotFrom)
	}
	if strings.Join(gotTo, ",") != "ops@example.com,dev@example.com" {
		t.Fatalf("to = %#v, want trimmed recipients", gotTo)
	}
	for _, want := range []string{
		"From: admin@example.com\r\n",
		"To: ops@example.com, dev@example.com\r\n",
		"Subject: Notice enabled\r\n",
		"Maintenance window tonight",
	} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("message body %q does not contain %q", gotBody, want)
		}
	}
}

func TestSMTPSenderSendReturnsTransportError(t *testing.T) {
	transportErr := errors.New("smtp unavailable")
	sender := NewSMTPSender(SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		Sender:   "admin@example.com",
	}, func(context.Context, string, smtp.Auth, string, []string, []byte) error {
		return transportErr
	})

	err := sender.Send(context.Background(), Message{
		To:      []string{"ops@example.com"},
		Subject: "Notice enabled",
		Body:    "Maintenance",
	})
	if !errors.Is(err, transportErr) {
		t.Fatalf("Send() error = %v, want transport error", err)
	}
}

func TestSMTPSenderSendRejectsConflictingTLSModes(t *testing.T) {
	calls := 0
	sender := NewSMTPSender(SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		Sender:   "admin@example.com",
		UseTLS:   true,
		StartTLS: true,
	}, func(context.Context, string, smtp.Auth, string, []string, []byte) error {
		calls++
		return nil
	})

	err := sender.Send(context.Background(), Message{
		To:      []string{"ops@example.com"},
		Subject: "Notice enabled",
		Body:    "Maintenance",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Send() error = %v, want ErrInvalidConfig", err)
	}
	if calls != 0 {
		t.Fatalf("transport calls = %d, want 0", calls)
	}
}

func TestSMTPSenderSendHonorsCanceledContextBeforeDial(t *testing.T) {
	sender := NewSMTPSender(SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		Sender:   "admin@example.com",
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.Send(ctx, Message{
		To:      []string{"ops@example.com"},
		Subject: "Notice enabled",
		Body:    "Maintenance",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v, want context.Canceled", err)
	}
}

func TestSMTPTransportDefaultsToPlainDialWhenTLSDisabled(t *testing.T) {
	fake := newFakeSMTPTransport()
	config := SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 25,
		Username: "smtp-user",
		Password: "smtp-password",
		Sender:   "admin@example.com",
	}

	err := sendSMTPWithDeps(
		context.Background(),
		config,
		smtpAuth(config),
		"admin@example.com",
		[]string{"ops@example.com"},
		[]byte("hello"),
		fake.deps(),
	)
	if err != nil {
		t.Fatalf("sendSMTPWithDeps() error = %v", err)
	}

	if fake.plainDialAddr != "smtp.example.com:25" {
		t.Fatalf("plain dial addr = %q, want smtp.example.com:25", fake.plainDialAddr)
	}
	if fake.tlsDialAddr != "" {
		t.Fatalf("tls dial addr = %q, want empty for plain SMTP", fake.tlsDialAddr)
	}
	if fake.client.startTLSServerName != "" {
		t.Fatalf("STARTTLS server name = %q, want empty for plain SMTP", fake.client.startTLSServerName)
	}
	wantCalls := []string{
		"extension:AUTH",
		"auth",
		"mail:admin@example.com",
		"rcpt:ops@example.com",
		"data",
		"data-close",
		"quit",
		"close",
	}
	if !reflect.DeepEqual(fake.client.calls, wantCalls) {
		t.Fatalf("client calls = %#v, want %#v", fake.client.calls, wantCalls)
	}
}

func TestSMTPTransportUsesImplicitTLSDial(t *testing.T) {
	fake := newFakeSMTPTransport()
	config := SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 465,
		Username: "smtp-user",
		Password: "smtp-password",
		Sender:   "admin@example.com",
		UseTLS:   true,
	}

	err := sendSMTPWithDeps(
		context.Background(),
		config,
		smtpAuth(config),
		"admin@example.com",
		[]string{"ops@example.com"},
		[]byte("hello"),
		fake.deps(),
	)
	if err != nil {
		t.Fatalf("sendSMTPWithDeps() error = %v", err)
	}

	if fake.tlsDialAddr != "smtp.example.com:465" {
		t.Fatalf("tls dial addr = %q, want smtp.example.com:465", fake.tlsDialAddr)
	}
	if fake.plainDialAddr != "" {
		t.Fatalf("plain dial addr = %q, want empty for implicit TLS", fake.plainDialAddr)
	}
	if fake.tlsServerName != "smtp.example.com" {
		t.Fatalf("tls server name = %q, want smtp.example.com", fake.tlsServerName)
	}
	if fake.client.startTLSServerName != "" {
		t.Fatalf("STARTTLS server name = %q, want empty for implicit TLS", fake.client.startTLSServerName)
	}
}

func TestSMTPTransportStartsTLSBeforeAuthAndEnvelope(t *testing.T) {
	fake := newFakeSMTPTransport()
	config := SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		Username: "smtp-user",
		Password: "smtp-password",
		Sender:   "admin@example.com",
		StartTLS: true,
	}

	err := sendSMTPWithDeps(
		context.Background(),
		config,
		smtpAuth(config),
		"admin@example.com",
		[]string{"ops@example.com"},
		[]byte("hello"),
		fake.deps(),
	)
	if err != nil {
		t.Fatalf("sendSMTPWithDeps() error = %v", err)
	}

	if fake.plainDialAddr != "smtp.example.com:587" {
		t.Fatalf("plain dial addr = %q, want smtp.example.com:587", fake.plainDialAddr)
	}
	if fake.tlsDialAddr != "" {
		t.Fatalf("tls dial addr = %q, want empty for STARTTLS", fake.tlsDialAddr)
	}
	wantCalls := []string{
		"starttls:smtp.example.com",
		"extension:AUTH",
		"auth",
		"mail:admin@example.com",
		"rcpt:ops@example.com",
		"data",
		"data-close",
		"quit",
		"close",
	}
	if !reflect.DeepEqual(fake.client.calls, wantCalls) {
		t.Fatalf("client calls = %#v, want %#v", fake.client.calls, wantCalls)
	}
}

func TestSMTPTransportFailsClosedWhenCredentialsConfiguredButAuthUnsupported(t *testing.T) {
	fake := newFakeSMTPTransport()
	fake.client.authSupported = false
	config := SMTPConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.com",
		SMTPPort: 25,
		Username: "smtp-user",
		Password: "smtp-password",
		Sender:   "admin@example.com",
	}

	err := sendSMTPWithDeps(
		context.Background(),
		config,
		smtpAuth(config),
		"admin@example.com",
		[]string{"ops@example.com"},
		[]byte("hello"),
		fake.deps(),
	)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("sendSMTPWithDeps() error = %v, want ErrInvalidConfig", err)
	}

	wantCalls := []string{"extension:AUTH", "close"}
	if !reflect.DeepEqual(fake.client.calls, wantCalls) {
		t.Fatalf("client calls = %#v, want %#v", fake.client.calls, wantCalls)
	}
}

type fakeSMTPTransport struct {
	plainDialAddr string
	tlsDialAddr   string
	tlsServerName string
	newClientHost string
	client        *fakeSMTPClient
}

func newFakeSMTPTransport() *fakeSMTPTransport {
	return &fakeSMTPTransport{client: &fakeSMTPClient{authSupported: true}}
}

func (f *fakeSMTPTransport) deps() smtpTransportDeps {
	return smtpTransportDeps{
		dialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			f.plainDialAddr = addr
			return fakeNetConn{}, nil
		},
		tlsDialContext: func(ctx context.Context, network string, addr string, config *tls.Config) (net.Conn, error) {
			f.tlsDialAddr = addr
			f.tlsServerName = config.ServerName
			return fakeNetConn{}, nil
		},
		newClient: func(conn net.Conn, host string) (smtpClient, error) {
			f.newClientHost = host
			return f.client, nil
		},
		now: func() time.Time {
			return time.Unix(1000, 0)
		},
	}
}

type fakeSMTPClient struct {
	calls              []string
	startTLSServerName string
	authSupported      bool
}

func (f *fakeSMTPClient) Extension(ext string) (bool, string) {
	f.calls = append(f.calls, "extension:"+ext)
	if ext == "AUTH" {
		return f.authSupported, ""
	}
	return false, ""
}

func (f *fakeSMTPClient) StartTLS(config *tls.Config) error {
	f.startTLSServerName = config.ServerName
	f.calls = append(f.calls, "starttls:"+config.ServerName)
	return nil
}

func (f *fakeSMTPClient) Auth(auth smtp.Auth) error {
	f.calls = append(f.calls, "auth")
	return nil
}

func (f *fakeSMTPClient) Mail(from string) error {
	f.calls = append(f.calls, "mail:"+from)
	return nil
}

func (f *fakeSMTPClient) Rcpt(to string) error {
	f.calls = append(f.calls, "rcpt:"+to)
	return nil
}

func (f *fakeSMTPClient) Data() (io.WriteCloser, error) {
	f.calls = append(f.calls, "data")
	return &fakeDataWriter{client: f}, nil
}

func (f *fakeSMTPClient) Quit() error {
	f.calls = append(f.calls, "quit")
	return nil
}

func (f *fakeSMTPClient) Close() error {
	f.calls = append(f.calls, "close")
	return nil
}

type fakeDataWriter struct {
	client *fakeSMTPClient
}

func (w *fakeDataWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *fakeDataWriter) Close() error {
	w.client.calls = append(w.client.calls, "data-close")
	return nil
}

type fakeNetConn struct{}

func (fakeNetConn) Read(b []byte) (int, error)         { return 0, io.EOF }
func (fakeNetConn) Write(b []byte) (int, error)        { return len(b), nil }
func (fakeNetConn) Close() error                       { return nil }
func (fakeNetConn) LocalAddr() net.Addr                { return fakeAddr("local") }
func (fakeNetConn) RemoteAddr() net.Addr               { return fakeAddr("remote") }
func (fakeNetConn) SetDeadline(t time.Time) error      { return nil }
func (fakeNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (fakeNetConn) SetWriteDeadline(t time.Time) error { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
