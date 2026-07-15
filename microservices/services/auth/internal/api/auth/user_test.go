package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/auth/internal/pkg/logger"
	"github.com/go-admin-kit/services/auth/internal/service/system"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRecordOnlineUserAsyncUsesDetachedTimeoutContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	fake := &fakeOnlineUserWriter{
		called: make(chan context.Context, 1),
		done:   make(chan struct{}),
	}
	api := &UserAPI{onlineUserService: fake}

	api.recordOnlineUserAsync(parent, system.OnlineUser{TokenID: "token-a"}, time.Hour)

	ctx := receiveOnlineUserContext(t, fake.called)
	if err := ctx.Err(); err != nil {
		t.Fatalf("online user context was canceled by parent context: %v", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("online user context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > onlineUserWriteTimeout {
		t.Fatalf("online user context deadline = %s from now, want within %s", remaining, onlineUserWriteTimeout)
	}

	close(fake.done)
}

func TestRecordOnlineUserAsyncLogsFailureAndReturnsImmediately(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	previousLogger := logger.Logger
	logger.Logger = zap.New(core)
	t.Cleanup(func() {
		logger.Logger = previousLogger
	})

	fake := &fakeOnlineUserWriter{
		err:    errors.New("redis unavailable"),
		called: make(chan context.Context, 1),
		done:   make(chan struct{}),
	}
	api := &UserAPI{onlineUserService: fake}

	returned := make(chan struct{})
	go func() {
		api.recordOnlineUserAsync(context.Background(), system.OnlineUser{TokenID: "token-a"}, time.Hour)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recordOnlineUserAsync blocked on online user write")
	}

	_ = receiveOnlineUserContext(t, fake.called)
	close(fake.done)

	deadline := time.After(time.Second)
	for logs.Len() == 0 {
		select {
		case <-deadline:
			t.Fatal("expected failure log for online user write")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	entry := logs.All()[0]
	if entry.Message != "failed to record online user" {
		t.Fatalf("log message = %q, want %q", entry.Message, "failed to record online user")
	}
}

func receiveOnlineUserContext(t *testing.T, ch <-chan context.Context) context.Context {
	t.Helper()

	select {
	case ctx := <-ch:
		return ctx
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for online user write")
		return nil
	}
}

type fakeOnlineUserWriter struct {
	err    error
	called chan context.Context
	done   chan struct{}
}

func (f *fakeOnlineUserWriter) SetOnlineUserContext(ctx context.Context, _ system.OnlineUser, _ time.Duration) error {
	f.called <- ctx
	if f.done != nil {
		<-f.done
	}
	return f.err
}

func (f *fakeOnlineUserWriter) RemoveOnlineUserContext(context.Context, string) error {
	return nil
}
