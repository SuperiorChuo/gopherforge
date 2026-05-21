package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestLoginLimitContextMethodsHonorCanceledContext(t *testing.T) {
	store := setupRateLimitTestRedis(t)

	cfg := LoginLimitConfig{
		Window:       time.Minute,
		MaxFailures:  1,
		LockDuration: time.Minute,
		KeyPrefix:    "unit_login_limit",
	}
	identifier := "admin:192.0.2.1"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	RecordLoginFailureContext(ctx, identifier, cfg)
	if store.Exists(fmt.Sprintf("%s:%s", cfg.KeyPrefix, identifier)) {
		t.Fatal("RecordLoginFailureContext should not write when context is canceled")
	}

	locked, ttl := IsLoginLockedContext(ctx, identifier, cfg)
	if locked || ttl != 0 {
		t.Fatalf("IsLoginLockedContext = (%v, %s), want unlocked with zero ttl", locked, ttl)
	}

	ClearLoginLimitContext(ctx, identifier, cfg)
}

func TestLoginLimiterWithClientUsesInjectedClient(t *testing.T) {
	globalStore := setupRateLimitTestRedis(t)

	injectedStore, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start injected miniredis: %v", err)
	}
	injectedClient := goredis.NewClient(&goredis.Options{Addr: injectedStore.Addr()})
	t.Cleanup(func() {
		_ = injectedClient.Close()
		injectedStore.Close()
	})

	cfg := LoginLimitConfig{
		Window:       time.Minute,
		MaxFailures:  2,
		LockDuration: time.Minute,
		KeyPrefix:    "unit_login_limit_injected",
	}
	identifier := "admin:192.0.2.2"
	key := fmt.Sprintf("%s:%s", cfg.KeyPrefix, identifier)
	lockKey := fmt.Sprintf("%s:lock", key)

	limiter := NewLoginLimiterWithClient(injectedClient)
	limiter.RecordFailureContext(context.Background(), identifier, cfg)

	if !injectedStore.Exists(key) {
		t.Fatalf("injected login limit key %q was not written", key)
	}
	if globalStore.Exists(key) || globalStore.Exists(lockKey) {
		t.Fatal("global redis was written; expected injected client only")
	}

	injectedStore.Set(lockKey, "1")
	injectedStore.SetTTL(lockKey, time.Minute)

	locked, ttl := limiter.IsLockedContext(context.Background(), identifier, cfg)
	if !locked || ttl <= 0 {
		t.Fatalf("IsLockedContext = (%v, %s), want locked with positive ttl", locked, ttl)
	}

	limiter.ClearContext(context.Background(), identifier, cfg)
	if injectedStore.Exists(key) || injectedStore.Exists(lockKey) {
		t.Fatal("ClearContext did not remove injected login limit keys")
	}
}
