// Package events publishes best-effort authentication domain events to NATS.
//
// Publishing must never fail or block the login path: the publisher is
// nil-safe, publishes asynchronously with a bounded timeout, and only logs a
// warning when delivery fails.
package events

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/go-admin-kit/services/auth/internal/pkg/logger"
	"github.com/nats-io/nats.go"
)

// NATS subjects for auth events.
const (
	SubjectLoginSuccess = "auth.login.success"
	SubjectLoginFailed  = "auth.login.failed"
	SubjectLogout       = "auth.logout"
)

// Login types recorded on auth.login.success events.
const (
	LoginTypeAccount     = "account"
	LoginTypeTOTP        = "totp"
	LoginTypeConsole     = "console"
	LoginTypeOAuthGithub = "oauth:github"
	LoginTypeOAuthWechat = "oauth:wechat"
)

const publishTimeout = 2 * time.Second

// LoginSuccessEvent is published on subject auth.login.success.
type LoginSuccessEvent struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	TenantID  uint   `json:"tenant_id"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	LoginType string `json:"login_type"`
	Timestamp string `json:"timestamp"`
}

// LoginFailedEvent is published on subject auth.login.failed.
type LoginFailedEvent struct {
	Username  string `json:"username"`
	TenantID  uint   `json:"tenant_id"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// LogoutEvent is published on subject auth.logout.
type LogoutEvent struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	Timestamp string `json:"timestamp"`
}

// Transport is the minimal publish surface the Publisher needs. *nats.Conn
// satisfies it; tests inject fakes.
type Transport interface {
	Publish(subject string, data []byte) error
}

// Publisher publishes JSON events to NATS. A nil *Publisher is a no-op, so
// callers never need to guard against a disabled event bus.
type Publisher struct {
	transport Transport
	closer    func()
	timeout   time.Duration
}

// Connect dials NATS and returns a Publisher. An empty URL disables event
// publishing entirely and returns (nil, nil).
func Connect(url string) (*Publisher, error) {
	if url == "" {
		return nil, nil
	}
	conn, err := nats.Connect(url,
		nats.Timeout(publishTimeout),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, err
	}
	return &Publisher{
		transport: conn,
		closer:    conn.Close,
		timeout:   publishTimeout,
	}, nil
}

// NewPublisherWithTransport builds a Publisher over an injected transport
// (used by tests). A nil transport yields a nil, no-op Publisher.
func NewPublisherWithTransport(transport Transport) *Publisher {
	if transport == nil {
		return nil
	}
	return &Publisher{transport: transport, timeout: publishTimeout}
}

// Close releases the underlying connection, if any.
func (p *Publisher) Close() {
	if p == nil || p.closer == nil {
		return
	}
	p.closer()
}

// PublishLoginSuccess publishes an auth.login.success event.
func (p *Publisher) PublishLoginSuccess(event LoginSuccessEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	p.publish(SubjectLoginSuccess, event)
}

// PublishLoginFailed publishes an auth.login.failed event.
func (p *Publisher) PublishLoginFailed(event LoginFailedEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	p.publish(SubjectLoginFailed, event)
}

// PublishLogout publishes an auth.logout event.
func (p *Publisher) PublishLogout(event LogoutEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	p.publish(SubjectLogout, event)
}

// publish delivers the event asynchronously and best-effort: it never blocks
// the caller beyond spawning a goroutine, never returns an error, and gives
// up after the publish timeout.
func (p *Publisher) publish(subject string, event any) {
	if p == nil || p.transport == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		warn("failed to marshal auth event", subject, err)
		return
	}

	transport := p.transport
	timeout := p.timeout
	if timeout <= 0 {
		timeout = publishTimeout
	}
	go func() {
		done := make(chan error, 1)
		go func() { done <- transport.Publish(subject, payload) }()

		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-done:
			if err != nil {
				warn("failed to publish auth event", subject, err)
			}
		case <-timer.C:
			warn("timed out publishing auth event", subject, nil)
		}
	}()
}

func warn(message, subject string, err error) {
	if logger.Logger == nil {
		return
	}
	if err != nil {
		logger.Warn(message, logger.String("subject", subject), logger.Err(err))
		return
	}
	logger.Warn(message, logger.String("subject", subject))
}

var (
	defaultMu        sync.RWMutex
	defaultPublisher *Publisher
)

// SetDefault installs the process-wide publisher used by the API handlers and
// returns a restore function. A nil publisher disables publishing.
func SetDefault(p *Publisher) func() {
	defaultMu.Lock()
	previous := defaultPublisher
	defaultPublisher = p
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		defaultPublisher = previous
		defaultMu.Unlock()
	}
}

// Default returns the process-wide publisher (possibly nil, which is safe).
func Default() *Publisher {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultPublisher
}
