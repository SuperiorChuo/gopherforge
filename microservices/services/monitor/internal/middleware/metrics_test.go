package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestMetricsRecordsSnapshotAndPrometheusOutput(t *testing.T) {
	resetMetricsForTest(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(Metrics())
	router.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.GET("/boom", func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "boom")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	snapshot := MetricsSnapshot()
	if got := snapshot["total_requests"]; got != uint64(2) {
		t.Fatalf("total_requests = %v, want 2", got)
	}
	if got := snapshot["in_flight"]; got != int64(0) {
		t.Fatalf("in_flight = %v, want 0", got)
	}
	if got := snapshot["error_count"]; got != uint64(1) {
		t.Fatalf("error_count = %v, want 1", got)
	}

	prometheus := PrometheusMetrics()
	for _, expected := range []string{
		"go_admin_kit_http_requests_total 2",
		"go_admin_kit_http_in_flight_requests 0",
		"go_admin_kit_http_request_errors_total 1",
		`go_admin_kit_http_route_requests_total{method="GET",path="/ok"} 1`,
		`go_admin_kit_http_route_responses_total{method="GET",path="/boom",status="500"} 1`,
	} {
		if !strings.Contains(prometheus, expected) {
			t.Fatalf("PrometheusMetrics() missing %q in:\n%s", expected, prometheus)
		}
	}
}

func TestMetricsHotPathDoesNotTakeGlobalWriteLock(t *testing.T) {
	content, err := os.ReadFile("metrics.go")
	if err != nil {
		t.Fatalf("read metrics.go: %v", err)
	}

	source := string(content)
	start := strings.Index(source, "func Metrics() gin.HandlerFunc")
	if start == -1 {
		t.Fatal("Metrics function not found")
	}
	end := strings.Index(source[start:], "const httpStatusInternalServerError")
	if end == -1 {
		t.Fatal("Metrics function end marker not found")
	}

	metricsFunction := source[start : start+end]
	if strings.Contains(metricsFunction, "globalMetrics.mu.Lock()") {
		t.Fatal("Metrics hot path should not take the global metrics write lock")
	}
}

func TestMetricsUsesInjectedDatabasePoolStatsProvider(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	restore := SetMetricsDatabasePoolStatsProvider(stubDatabasePoolStatsProvider{
		stats: sql.DBStats{
			OpenConnections: 7,
			InUse:           3,
			Idle:            4,
			WaitCount:       11,
		},
	})
	t.Cleanup(restore)

	snapshot := MetricsSnapshot()
	pool, ok := snapshot["database_pool"].(gin.H)
	if !ok {
		t.Fatalf("database_pool = %T, want gin.H", snapshot["database_pool"])
	}
	if got := pool["status"]; got != "ok" {
		t.Fatalf("database_pool.status = %v, want ok", got)
	}
	if got := pool["open_connections"]; got != 7 {
		t.Fatalf("database_pool.open_connections = %v, want 7", got)
	}

	prometheus := PrometheusMetrics()
	for _, expected := range []string{
		"go_admin_kit_db_open_connections 7",
		"go_admin_kit_db_in_use_connections 3",
		"go_admin_kit_db_idle_connections 4",
		"go_admin_kit_db_wait_total 11",
	} {
		if !strings.Contains(prometheus, expected) {
			t.Fatalf("PrometheusMetrics() missing %q in:\n%s", expected, prometheus)
		}
	}
}

func resetMetricsForTest(t *testing.T) {
	t.Helper()

	oldMetrics := globalMetrics
	globalMetrics = newMetricsStore()
	t.Cleanup(func() {
		globalMetrics = oldMetrics
	})
}

type stubDatabasePoolStatsProvider struct {
	stats sql.DBStats
	err   error
}

func (s stubDatabasePoolStatsProvider) DatabaseStats() (sql.DBStats, error) {
	return s.stats, s.err
}
