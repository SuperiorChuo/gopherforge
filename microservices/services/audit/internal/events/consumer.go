// Package events consumes authentication domain events from NATS JetStream
// and persists them as login logs.
//
// The auth-service publishes best-effort core NATS messages; this package owns
// the durable side: it ensures the AUTH_EVENTS stream exists so matching
// events are captured, and runs a durable consumer so login logs survive
// restarts and are written at-least-once.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/logger"
)

// Stream and subject names mirror services/auth/internal/events.
const (
	StreamName      = "AUTH_EVENTS"
	StreamSubjects  = "auth.>"
	ConsumerDurable = "login-log-writer"

	SubjectLoginSuccess = "auth.login.success"
	SubjectLoginFailed  = "auth.login.failed"
)

// Canonical login_logs.login_type codes. Frontends render these labels; keep
// tdesign-vue-go and react-antd in sync when changing them.
const (
	LoginTypePassword int8 = 1 // account and console password logins
	LoginTypeGithub   int8 = 2
	LoginTypeWechat   int8 = 3
	LoginTypeTOTP     int8 = 4 // password + 2FA verification
)

const (
	loginStatusSuccess int8 = 1
	loginStatusFailed  int8 = 0

	recordTimeout   = 5 * time.Second
	setupTimeout    = 10 * time.Second
	setupRetryDelay = 5 * time.Second
	redeliveryDelay = 5 * time.Second
	maxDeliver      = 5
	streamMaxAge    = 7 * 24 * time.Hour
	messageMaxLen   = 255
)

// loginEvent is the superset of the auth-service success/failed payloads.
// TenantID is optional (older publishers omit it); zero falls back to default tenant 1.
type loginEvent struct {
	UserID    uint   `json:"user_id"`
	TenantID  uint   `json:"tenant_id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	LoginType string `json:"login_type"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// LoginLogRecorder is the persistence surface the consumer needs;
// *systemsvc.LoginLogService satisfies it.
type LoginLogRecorder interface {
	RecordContext(ctx context.Context, info *systemsvc.LoginInfo) error
}

// Consumer runs a durable JetStream consumer that writes login logs.
type Consumer struct {
	conn     *nats.Conn
	recorder LoginLogRecorder

	mu         sync.Mutex
	consumeCtx jetstream.ConsumeContext
}

// StartLoginLogConsumer connects to NATS and begins consuming auth login
// events. An empty URL disables consumption entirely and returns (nil, nil).
// Stream/consumer setup retries in the background until it succeeds or ctx is
// cancelled, so a temporarily unavailable NATS server only delays consumption.
func StartLoginLogConsumer(ctx context.Context, url string, recorder LoginLogRecorder) (*Consumer, error) {
	if url == "" {
		return nil, nil
	}
	conn, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	c := &Consumer{conn: conn, recorder: recorder}
	go c.run(ctx)
	return c, nil
}

// Close stops consumption and releases the connection. Unacked in-flight
// messages are redelivered on next start via the durable consumer.
func (c *Consumer) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	consumeCtx := c.consumeCtx
	c.consumeCtx = nil
	c.mu.Unlock()
	if consumeCtx != nil {
		consumeCtx.Stop()
	}
	c.conn.Close()
}

func (c *Consumer) run(ctx context.Context) {
	for {
		err := c.setup(ctx)
		if err == nil {
			return
		}
		warn("auth event consumer setup failed, retrying", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(setupRetryDelay):
		}
	}
}

// setup ensures the stream and durable consumer exist, then starts delivery.
func (c *Consumer) setup(ctx context.Context) error {
	js, err := jetstream.New(c.conn)
	if err != nil {
		return fmt.Errorf("jetstream init: %w", err)
	}

	ensureCtx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()

	stream, err := js.CreateOrUpdateStream(ensureCtx, jetstream.StreamConfig{
		Name:        StreamName,
		Description: "Authentication domain events published by auth-service",
		Subjects:    []string{StreamSubjects},
		Storage:     jetstream.FileStorage,
		Retention:   jetstream.LimitsPolicy,
		Discard:     jetstream.DiscardOld,
		MaxAge:      streamMaxAge,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", StreamName, err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ensureCtx, jetstream.ConsumerConfig{
		Durable:        ConsumerDurable,
		Description:    "Persists auth login events into login_logs",
		FilterSubjects: []string{SubjectLoginSuccess, SubjectLoginFailed},
		AckPolicy:      jetstream.AckExplicitPolicy,
		AckWait:        30 * time.Second,
		MaxDeliver:     maxDeliver,
	})
	if err != nil {
		return fmt.Errorf("ensure consumer %s: %w", ConsumerDurable, err)
	}

	consumeCtx, err := consumer.Consume(c.handle)
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}

	c.mu.Lock()
	c.consumeCtx = consumeCtx
	c.mu.Unlock()
	info("auth event consumer started", StreamName)
	return nil
}

// handle persists one event: malformed payloads are terminated (poison
// messages must not block the stream), transient DB failures are redelivered
// with a delay, and successful writes are acked.
func (c *Consumer) handle(msg jetstream.Msg) {
	loginInfo, err := buildLoginInfo(msg.Subject(), msg.Data())
	if err != nil {
		warn("terminating malformed auth event", err)
		_ = msg.Term()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	if err := c.recorder.RecordContext(ctx, loginInfo); err != nil {
		warn("login log write failed, scheduling redelivery", err)
		_ = msg.NakWithDelay(redeliveryDelay)
		return
	}
	_ = msg.Ack()
}

// buildLoginInfo maps an auth event payload onto the login_logs write model.
func buildLoginInfo(subject string, data []byte) (*systemsvc.LoginInfo, error) {
	var event loginEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", subject, err)
	}

	tenantID := event.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	info := &systemsvc.LoginInfo{
		UserID:     event.UserID,
		TenantID:   tenantID,
		Username:   event.Username,
		LoginType:  loginTypeCode(event.LoginType),
		IP:         event.IP,
		UserAgent:  event.UserAgent,
		OccurredAt: parseEventTime(event.Timestamp),
	}

	switch subject {
	case SubjectLoginSuccess:
		info.Status = loginStatusSuccess
	case SubjectLoginFailed:
		info.Status = loginStatusFailed
		info.Message = truncate(event.Reason, messageMaxLen)
	default:
		return nil, fmt.Errorf("unexpected subject %q", subject)
	}
	return info, nil
}

// loginTypeCode maps auth-service login type strings to login_logs codes.
// Unknown strings fall back to password so a new publisher value degrades
// gracefully instead of poisoning the message.
func loginTypeCode(loginType string) int8 {
	switch strings.ToLower(loginType) {
	case "oauth:github":
		return LoginTypeGithub
	case "oauth:wechat":
		return LoginTypeWechat
	case "totp":
		return LoginTypeTOTP
	default: // "account", "console", failures (no login_type), future values
		return LoginTypePassword
	}
}

func parseEventTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func info(message, stream string) {
	if logger.Logger == nil {
		return
	}
	logger.Info(message, logger.String("stream", stream))
}

func warn(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Warn(message, logger.Err(err))
}
