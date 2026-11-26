package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
)

type stubLoginRedis struct {
	values map[string]string
	counts map[string]int64
	ttls   map[string]time.Duration
}

func newStubLoginRedis() *stubLoginRedis {
	return &stubLoginRedis{
		values: map[string]string{},
		counts: map[string]int64{},
		ttls:   map[string]time.Duration{},
	}
}

func (s *stubLoginRedis) Get(ctx context.Context, key string) *goredis.StringCmd {
	val, ok := s.values[key]
	if !ok {
		return goredis.NewStringResult("", goredis.Nil)
	}
	return goredis.NewStringResult(val, nil)
}

func (s *stubLoginRedis) TTL(ctx context.Context, key string) *goredis.DurationCmd {
	return goredis.NewDurationResult(s.ttls[key], nil)
}

func (s *stubLoginRedis) Incr(ctx context.Context, key string) *goredis.IntCmd {
	s.counts[key]++
	return goredis.NewIntResult(s.counts[key], nil)
}

func (s *stubLoginRedis) Expire(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd {
	s.ttls[key] = expiration
	return goredis.NewBoolResult(true, nil)
}

func (s *stubLoginRedis) Set(ctx context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd {
	s.values[key] = value.(string)
	s.ttls[key] = expiration
	return goredis.NewStatusResult("OK", nil)
}

func (s *stubLoginRedis) Del(ctx context.Context, keys ...string) *goredis.IntCmd {
	var deleted int64
	for _, k := range keys {
		delete(s.values, k)
		delete(s.counts, k)
		delete(s.ttls, k)
		deleted++
	}
	return goredis.NewIntResult(deleted, nil)
}

func TestLoginLimiterLifecycleAndCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newStubLoginRedis()
	limiter := NewLoginLimiterWithClient(store)
	cfg := LoginLimitConfigFromPolicy(sharedruntimeconfig.SecurityPolicy{
		LoginLimitMaxFailures:      2,
		LoginLimitWindowMinutes:    5,
		LoginLimitLockMinutes:      10,
		LoginIPShieldMaxFailures:   20,
		LoginIPShieldWindowMinutes: 10,
		LoginIPShieldBlockMinutes:  15,
	})

	id := LoginIdentifier(" Alice ", "127.0.0.1")
	if id != "alice:127.0.0.1" {
		t.Fatalf("LoginIdentifier = %q", id)
	}

	ctx := context.Background()
	limiter.RecordFailureContext(ctx, id, cfg)
	if locked, _ := limiter.IsLockedContext(ctx, id, cfg); locked {
		t.Fatalf("should not lock after 1 failure")
	}
	limiter.RecordFailureContext(ctx, id, cfg)
	if locked, ttl := limiter.IsLockedContext(ctx, id, cfg); !locked || ttl != 10*time.Minute {
		t.Fatalf("expected locked with 10m TTL, got locked=%v ttl=%v", locked, ttl)
	}

	// Verify Check middleware rejects locked IP
	limiter.RecordFailureContext(ctx, "127.0.0.1", cfg)
	limiter.RecordFailureContext(ctx, "127.0.0.1", cfg)
	r := gin.New()
	r.POST("/login", limiter.Check(cfg), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("Check status = %d, want 429", rec.Code)
	}

	limiter.ClearContext(ctx, id, cfg)
	if locked, _ := limiter.IsLockedContext(ctx, id, cfg); locked {
		t.Fatalf("expected unlocked after ClearContext")
	}

	ipCfg := IPFailureConfigFromLoginLimit(cfg)
	if ipCfg.MaxFailures != 20 || ipCfg.BlockMinutes != 15*time.Minute {
		t.Fatalf("IPFailureConfigFromLoginLimit = %+v", ipCfg)
	}
}
