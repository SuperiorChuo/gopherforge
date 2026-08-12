package graceful

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
)

func init() {
	logger.InitLogger("", "error", 0, 0, 0)
}

func TestShutdownOrderLIFO(t *testing.T) {
	s := New(WithTimeout(5 * time.Second))
	var order []string
	var mu sync.Mutex
	appendOrder := func(name string) {
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}
	// 注册顺序：底层 → 入口；关闭应逆序
	s.Register("database", func(ctx context.Context) error {
		appendOrder("database")
		return nil
	})
	s.Register("workers", func(ctx context.Context) error {
		appendOrder("workers")
		return nil
	})
	s.Register("http", func(ctx context.Context) error {
		appendOrder("http")
		return nil
	})
	if err := s.Shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"http", "workers", "database"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("order = %v, want %v (LIFO)", order, want)
	}
}

func TestShutdownError(t *testing.T) {
	s := New(WithTimeout(5 * time.Second))
	s.Register("ok", func(ctx context.Context) error { return nil })
	s.Register("fail", func(ctx context.Context) error {
		return errors.New("shutdown failed")
	})
	err := s.Shutdown()
	if err == nil {
		t.Fatal("expected error from shutdown")
	}
}

func TestShutdownTimeout(t *testing.T) {
	s := New(WithTimeout(100 * time.Millisecond), WithHardTimeout(2*time.Second))
	s.Register("slow", func(ctx context.Context) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	start := time.Now()
	err := s.Shutdown()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("shutdown took %v, want < 2s", elapsed)
	}
}

func TestHandlerNames(t *testing.T) {
	s := New()
	s.Register("a", func(ctx context.Context) error { return nil })
	s.Register("b", func(ctx context.Context) error { return nil })
	names := s.HandlerNames()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("names = %v, want [a b] registration order", names)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	s := New()
	var count int32
	s.Register("dup", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	s.Register("dup", func(ctx context.Context) error {
		atomic.AddInt32(&count, 100)
		return nil
	})
	_ = s.Shutdown()
	if atomic.LoadInt32(&count) != 100 {
		t.Fatalf("count = %d, want 100 (should use latest)", count)
	}
}

func TestNilLoggerSafe(t *testing.T) {
	prev := logger.Logger
	logger.Logger = nil
	defer func() { logger.Logger = prev }()

	s := New(WithTimeout(time.Second))
	s.Register("ok", func(ctx context.Context) error { return nil })
	if err := s.Shutdown(); err != nil {
		t.Fatalf("shutdown with nil logger: %v", err)
	}
}
