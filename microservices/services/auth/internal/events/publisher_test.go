package events

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingTransport struct {
	mu       sync.Mutex
	messages map[string][]byte
	err      error
	done     chan struct{}
}

func newRecordingTransport() *recordingTransport {
	return &recordingTransport{
		messages: make(map[string][]byte),
		done:     make(chan struct{}, 8),
	}
}

func (r *recordingTransport) Publish(subject string, data []byte) error {
	r.mu.Lock()
	r.messages[subject] = data
	r.mu.Unlock()
	r.done <- struct{}{}
	return r.err
}

func (r *recordingTransport) waitForPublish(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async publish")
	}
}

func (r *recordingTransport) payload(subject string) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.messages[subject]
}

func TestNilPublisherIsSafe(t *testing.T) {
	var p *Publisher
	p.PublishLoginSuccess(LoginSuccessEvent{Username: "alice"})
	p.PublishLoginFailed(LoginFailedEvent{Username: "alice"})
	p.PublishLogout(LogoutEvent{Username: "alice"})
	p.Close()
}

func TestNewPublisherWithNilTransportIsNoOp(t *testing.T) {
	if p := NewPublisherWithTransport(nil); p != nil {
		t.Fatalf("NewPublisherWithTransport(nil) = %v, want nil", p)
	}
}

func TestPublishLoginSuccessDeliversJSONPayload(t *testing.T) {
	transport := newRecordingTransport()
	p := NewPublisherWithTransport(transport)

	p.PublishLoginSuccess(LoginSuccessEvent{
		UserID:    42,
		Username:  "alice",
		IP:        "127.0.0.1",
		UserAgent: "go-test",
		LoginType: LoginTypeAccount,
	})
	transport.waitForPublish(t)

	var event LoginSuccessEvent
	if err := json.Unmarshal(transport.payload(SubjectLoginSuccess), &event); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if event.UserID != 42 || event.Username != "alice" || event.LoginType != LoginTypeAccount {
		t.Fatalf("event = %+v, want user 42/alice with account login type", event)
	}
	if event.Timestamp == "" {
		t.Fatal("event timestamp should be auto-filled")
	}
	if _, err := time.Parse(time.RFC3339, event.Timestamp); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", event.Timestamp, err)
	}
}

func TestPublishLoginFailedDeliversReason(t *testing.T) {
	transport := newRecordingTransport()
	p := NewPublisherWithTransport(transport)

	p.PublishLoginFailed(LoginFailedEvent{Username: "alice", Reason: "invalid_credentials"})
	transport.waitForPublish(t)

	var event LoginFailedEvent
	if err := json.Unmarshal(transport.payload(SubjectLoginFailed), &event); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if event.Reason != "invalid_credentials" {
		t.Fatalf("reason = %q, want invalid_credentials", event.Reason)
	}
}

func TestPublishLogoutDeliversEvent(t *testing.T) {
	transport := newRecordingTransport()
	p := NewPublisherWithTransport(transport)

	p.PublishLogout(LogoutEvent{UserID: 42, Username: "alice"})
	transport.waitForPublish(t)

	var event LogoutEvent
	if err := json.Unmarshal(transport.payload(SubjectLogout), &event); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if event.UserID != 42 || event.Username != "alice" {
		t.Fatalf("event = %+v, want user 42/alice", event)
	}
}

func TestPublishFailureDoesNotPropagate(t *testing.T) {
	transport := newRecordingTransport()
	transport.err = errors.New("nats unavailable")
	p := NewPublisherWithTransport(transport)

	// Must not panic or block the caller even when the transport errors.
	p.PublishLoginSuccess(LoginSuccessEvent{Username: "alice"})
	transport.waitForPublish(t)
}

func TestSetDefaultInstallsAndRestoresPublisher(t *testing.T) {
	transport := newRecordingTransport()
	p := NewPublisherWithTransport(transport)

	restore := SetDefault(p)
	if Default() != p {
		t.Fatal("Default() should return the installed publisher")
	}
	restore()
	if Default() == p {
		t.Fatal("restore should reinstate the previous publisher")
	}
}
