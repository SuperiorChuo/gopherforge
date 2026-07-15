package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
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

func TestLoginLimitConfigFromPolicy(t *testing.T) {
	cfg := LoginLimitConfigFromPolicy(runtimeconfig.SecurityPolicy{
		LoginLimitMaxFailures:   3,
		LoginLimitWindowMinutes: 7,
		LoginLimitLockMinutes:   11,
	})

	if cfg.MaxFailures != 3 || cfg.Window != 7*time.Minute || cfg.LockDuration != 11*time.Minute || cfg.KeyPrefix != "login_limit" {
		t.Fatalf("LoginLimitConfigFromPolicy() = %#v, want runtime policy values", cfg)
	}
}

func TestCheckLoginLimitUsesStableErrorCodeForLockedLogin(t *testing.T) {
	store := setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	cfg := LoginLimitConfig{
		Window:       time.Minute,
		MaxFailures:  1,
		LockDuration: time.Minute,
		KeyPrefix:    "unit_login_limit_check",
	}
	lockKey := fmt.Sprintf("%s:%s:lock", cfg.KeyPrefix, "admin")
	store.Set(lockKey, "1")
	store.SetTTL(lockKey, time.Minute)

	router := gin.New()
	router.Use(CheckLoginLimit(cfg))
	router.POST("/login", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	assertMiddlewareErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeLoginLocked)
}
