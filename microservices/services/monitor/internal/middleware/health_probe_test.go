package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
)

func TestIsHealthProbePath(t *testing.T) {
	cases := map[string]bool{
		"/api/v1/health/live":     true,
		"/api/v1/health/ready":    true,
		"/api/v1/im/health/ready": true,
		"/api/v1/users":           false,
		"/api/v1/health/readyz":   false,
	}
	for path, want := range cases {
		if got := isHealthProbePath(path); got != want {
			t.Errorf("isHealthProbePath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRateLimitBypassesHealthProbePaths(t *testing.T) {
	store := setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	cfg := RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 0,
		KeyPrefix:   "unit_probe_limit",
	}
	router := gin.New()
	router.Use(RateLimit(cfg))
	handler := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router.GET("/api/v1/health/ready", handler)
	router.GET("/api/v1/users", handler)

	probe := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	probe.RemoteAddr = "192.0.2.10:12345"
	probeRec := httptest.NewRecorder()
	router.ServeHTTP(probeRec, probe)
	if probeRec.Code != http.StatusNoContent {
		t.Fatalf("probe status = %d, want %d", probeRec.Code, http.StatusNoContent)
	}
	if store.Exists("unit_probe_limit:192.0.2.10") {
		t.Fatalf("probe request wrote rate limit key; probes must not touch Redis")
	}

	normal := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	normal.RemoteAddr = "192.0.2.10:12345"
	normalRec := httptest.NewRecorder()
	router.ServeHTTP(normalRec, normal)
	if normalRec.Code != http.StatusTooManyRequests {
		t.Fatalf("non-probe status = %d, want %d", normalRec.Code, http.StatusTooManyRequests)
	}
}

func TestDynamicRateLimitBypassesHealthProbePaths(t *testing.T) {
	store := setupRateLimitTestRedis(t)
	gin.SetMode(gin.TestMode)

	reader := stubRuntimePolicyReader{policy: runtimeconfig.SecurityPolicy{
		RateLimitEnabled:       true,
		RateLimitWindowSeconds: 60,
		RateLimitMaxRequests:   0,
	}}
	router := gin.New()
	router.Use(NewRateLimiter().DynamicMiddleware(reader))
	router.GET("/api/v1/health/live", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.RemoteAddr = "192.0.2.11:12345"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("probe status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if store.Exists("rate_limit:192.0.2.11") {
		t.Fatalf("dynamic probe request wrote rate limit key")
	}
}
