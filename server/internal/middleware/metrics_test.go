package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

func resetMetricsForTest(t *testing.T) {
	t.Helper()

	oldMetrics := globalMetrics
	globalMetrics = newMetricsStore()
	t.Cleanup(func() {
		globalMetrics = oldMetrics
	})
}
