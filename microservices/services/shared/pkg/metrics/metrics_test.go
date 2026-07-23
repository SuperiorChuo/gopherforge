package metrics

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestInstallServesPrometheusText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Install(r)
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ping status = %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"go_admin_kit_http_requests_total",
		"go_admin_kit_http_request_duration_seconds_bucket{le=\"0.005\"}",
		`go_admin_kit_http_route_requests_total{method="GET",path="/ping"}`,
		"go_admin_kit_go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
	if strings.Contains(body, "go_admin_kit_db_open_connections") {
		t.Error("db metrics rendered without SetDBStats")
	}
}

func TestSetDBStatsRendersPoolMetrics(t *testing.T) {
	SetDBStats(func() sql.DBStats {
		return sql.DBStats{OpenConnections: 3, InUse: 1, Idle: 2, WaitCount: 4, WaitDuration: 5 * time.Second}
	})
	defer SetDBStats(nil)

	body := Render()
	for _, want := range []string{
		"go_admin_kit_db_open_connections 3",
		"go_admin_kit_db_in_use_connections 1",
		"go_admin_kit_db_idle_connections 2",
		"go_admin_kit_db_wait_total 4",
		"go_admin_kit_db_wait_duration_seconds 5.000000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

func TestInstallDisabledByEnv(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "false")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Install(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", w.Code)
	}
}

func TestUnmatchedRouteBucketsCardinality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())

	for _, p := range []string{"/nope-1", "/nope-2", "/nope-3"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
	}
	body := Render()
	if strings.Contains(body, "nope-1") {
		t.Error("unmatched paths must not appear verbatim (cardinality guard)")
	}
	if !strings.Contains(body, `path="unmatched"`) {
		t.Error("expected unmatched bucket present")
	}
}
