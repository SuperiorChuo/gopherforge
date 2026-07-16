package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	redisstore "github.com/go-admin-kit/services/ai/internal/pkg/redis"
	"github.com/go-admin-kit/services/ai/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/shared/pkg/response"
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
	assertMiddlewareErrorCode(t, recorder.Body.Bytes(), response.ErrorCodeRateLimited)
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

func TestRateLimiterDynamicMiddlewareUsesRuntimePolicy(t *testing.T) {
	setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	reader := stubRuntimePolicyReader{policy: runtimeconfig.SecurityPolicy{
		RateLimitEnabled:       true,
		RateLimitWindowSeconds: 1,
		RateLimitMaxRequests:   1,
	}}

	router := gin.New()
	router.Use(NewRateLimiter().DynamicMiddleware(reader))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.3:12345"
	first := httptest.NewRecorder()
	router.ServeHTTP(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.3:12345"
	second := httptest.NewRecorder()
	router.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	assertMiddlewareErrorCode(t, second.Body.Bytes(), response.ErrorCodeRateLimited)
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

type stubRuntimePolicyReader struct {
	policy runtimeconfig.SecurityPolicy
}

func (s stubRuntimePolicyReader) SecurityPolicy(ctx context.Context) runtimeconfig.SecurityPolicy {
	return s.policy
}

func assertMiddlewareErrorCode(t *testing.T, body []byte, want response.ErrorCode) {
	t.Helper()

	var payload response.Response
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ErrorCode != want {
		t.Fatalf("error_code = %q, want %q; body=%s", payload.ErrorCode, want, string(body))
	}
}
