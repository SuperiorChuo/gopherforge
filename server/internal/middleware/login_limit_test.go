package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"
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
