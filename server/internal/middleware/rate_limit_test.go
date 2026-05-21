package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestRateLimitIncrementsBeforeRejectingOverLimitRequest(t *testing.T) {
	store := setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	cfg := RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 1,
		KeyPrefix:   "unit_rate_limit",
	}
	key := fmt.Sprintf("%s:%s", cfg.KeyPrefix, "192.0.2.1")
	store.Set(key, "1")

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("get rate limit count: %v", err)
	}
	if got != "2" {
		t.Fatalf("rate limit count = %q, want %q", got, "2")
	}
}

func TestRateLimiterWithClientUsesInjectedClient(t *testing.T) {
	globalStore := setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	injectedStore, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start injected miniredis: %v", err)
	}
	injectedClient := goredis.NewClient(&goredis.Options{Addr: injectedStore.Addr()})
	t.Cleanup(func() {
		_ = injectedClient.Close()
		injectedStore.Close()
	})

	cfg := RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 10,
		KeyPrefix:   "unit_rate_limit_injected",
	}
	key := fmt.Sprintf("%s:%s", cfg.KeyPrefix, "192.0.2.2")

	router := gin.New()
	router.Use(NewRateLimiterWithClient(injectedClient).Middleware(cfg))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if !injectedStore.Exists(key) {
		t.Fatalf("injected rate limit key %q was not written", key)
	}
	if globalStore.Exists(key) {
		t.Fatalf("global rate limit key %q was written; expected injected client only", key)
	}
}

func setupRateLimitTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})

	return store
}
