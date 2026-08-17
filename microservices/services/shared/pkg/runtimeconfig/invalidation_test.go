package runtimeconfig

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupInvalidationTestRedis(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	previousClient := redisstore.Client
	redisstore.Client = goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() {
		_ = redisstore.Client.Close()
		redisstore.Client = previousClient
		store.Close()
	})
}

func TestInvalidationHandlerPublishesOnlySupportedKeys(t *testing.T) {
	setupInvalidationTestRedis(t)

	var calls atomic.Int32
	handler := InvalidationHandler{
		IsSupported: func(key string) bool { return key == "security.policy" },
		Refresh: func(context.Context, string) error {
			calls.Add(1)
			return nil
		},
	}
	subscriber, err := handler.Start(context.Background())
	if err != nil {
		t.Fatalf("start handler: %v", err)
	}
	defer func() { _ = subscriber.Close() }()

	if err := handler.Publish(context.Background(), "unknown"); err != nil {
		t.Fatalf("unsupported publish: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("unsupported key refresh calls = %d, want 0", got)
	}
}

func TestInvalidationHandlerStartsAndRefreshesSupportedPayload(t *testing.T) {
	setupInvalidationTestRedis(t)

	var calls atomic.Int32
	handler := InvalidationHandler{
		IsSupported: func(key string) bool { return key == "security.policy" },
		Refresh: func(_ context.Context, key string) error {
			if key != "security.policy" {
				t.Errorf("refresh key = %q, want security.policy", key)
			}
			calls.Add(1)
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	subscriber, err := handler.Start(ctx)
	if err != nil {
		t.Fatalf("start handler: %v", err)
	}
	defer func() { _ = subscriber.Close() }()

	if err := handler.Publish(ctx, "security.policy"); err != nil {
		t.Fatalf("publish supported key: %v", err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for calls.Load() == 0 {
		select {
		case <-deadline.C:
			t.Fatal("supported payload was not refreshed")
		case <-time.After(5 * time.Millisecond):
		}
	}

	if err := handler.Publish(ctx, "unknown"); err != nil {
		t.Fatalf("publish unsupported key: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestInvalidationHandlerFailsClosedWithoutKeyFilter(t *testing.T) {
	setupInvalidationTestRedis(t)

	handler := InvalidationHandler{}
	if err := handler.Publish(context.Background(), "security.policy"); err != nil {
		t.Fatalf("nil filter publish: %v", err)
	}
}
