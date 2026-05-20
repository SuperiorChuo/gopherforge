package middleware

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

type routeMetric struct {
	Method string
	Path   string
}

type routeStatusMetric struct {
	Method string
	Path   string
	Status int
}

type routeStats struct {
	Count             uint64
	ErrorCount        uint64
	TotalLatencyNanos int64
	LatencyBuckets    []uint64
}

type metricsStore struct {
	mu                sync.RWMutex
	startedAt         time.Time
	totalRequests     atomic.Uint64
	inFlight          atomic.Int64
	totalLatencyNanos atomic.Int64
	errorCount        atomic.Uint64
	statusCounts      map[int]uint64
	routeCounts       map[routeMetric]uint64
	routeStatusCounts map[routeStatusMetric]uint64
	latencyBuckets    []atomic.Uint64
	routeStats        map[routeMetric]*routeStats
}

var latencyBucketSeconds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var globalMetrics = newMetricsStore()

func newMetricsStore() *metricsStore {
	return &metricsStore{
		startedAt:         time.Now(),
		statusCounts:      make(map[int]uint64),
		routeCounts:       make(map[routeMetric]uint64),
		routeStatusCounts: make(map[routeStatusMetric]uint64),
		latencyBuckets:    make([]atomic.Uint64, len(latencyBucketSeconds)),
		routeStats:        make(map[routeMetric]*routeStats),
	}
}

// Metrics records HTTP counters, errors, and latency histograms.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		globalMetrics.inFlight.Add(1)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		metric := routeMetric{Method: c.Request.Method, Path: path}
		statusMetric := routeStatusMetric{Method: c.Request.Method, Path: path, Status: status}
		isError := status >= httpStatusInternalServerError || len(c.Errors) > 0

		globalMetrics.totalRequests.Add(1)
		globalMetrics.inFlight.Add(-1)
		globalMetrics.totalLatencyNanos.Add(latency.Nanoseconds())
		if isError {
			globalMetrics.errorCount.Add(1)
		}
		globalMetrics.recordLatency(latency)

		globalMetrics.mu.Lock()
		globalMetrics.statusCounts[status]++
		globalMetrics.routeCounts[metric]++
		globalMetrics.routeStatusCounts[statusMetric]++
		stats := globalMetrics.routeStats[metric]
		if stats == nil {
			stats = &routeStats{LatencyBuckets: make([]uint64, len(latencyBucketSeconds))}
			globalMetrics.routeStats[metric] = stats
		}
		stats.Count++
		if isError {
			stats.ErrorCount++
		}
		stats.TotalLatencyNanos += latency.Nanoseconds()
		recordLatencyBucket(stats.LatencyBuckets, latency)
		globalMetrics.mu.Unlock()
	}
}

const httpStatusInternalServerError = 500

func (m *metricsStore) recordLatency(latency time.Duration) {
	recordAtomicLatencyBucket(m.latencyBuckets, latency)
}

func recordLatencyBucket(buckets []uint64, latency time.Duration) {
	seconds := latency.Seconds()
	for i, upperBound := range latencyBucketSeconds {
		if seconds <= upperBound {
			buckets[i]++
		}
	}
}

func recordAtomicLatencyBucket(buckets []atomic.Uint64, latency time.Duration) {
	seconds := latency.Seconds()
	for i, upperBound := range latencyBucketSeconds {
		if seconds <= upperBound {
			buckets[i].Add(1)
		}
	}
}

func MetricsSnapshot() gin.H {
	totalRequests := globalMetrics.totalRequests.Load()
	inFlight := globalMetrics.inFlight.Load()
	totalLatencyNanos := globalMetrics.totalLatencyNanos.Load()
	errorCount := globalMetrics.errorCount.Load()
	latencyBuckets := atomicLatencyBucketsSnapshot(globalMetrics.latencyBuckets)

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	statusCounts := make(map[int]uint64, len(globalMetrics.statusCounts))
	for status, count := range globalMetrics.statusCounts {
		statusCounts[status] = count
	}

	routeCounts := make(map[string]uint64, len(globalMetrics.routeCounts))
	for route, count := range globalMetrics.routeCounts {
		routeCounts[route.Method+" "+route.Path] = count
	}

	routeStatusCounts := make([]gin.H, 0, len(globalMetrics.routeStatusCounts))
	for route, count := range globalMetrics.routeStatusCounts {
		routeStatusCounts = append(routeStatusCounts, gin.H{
			"method": route.Method,
			"path":   route.Path,
			"status": route.Status,
			"count":  count,
		})
	}
	sort.Slice(routeStatusCounts, func(i, j int) bool {
		left := routeStatusCounts[i]
		right := routeStatusCounts[j]
		if left["path"] == right["path"] {
			if left["method"] == right["method"] {
				return left["status"].(int) < right["status"].(int)
			}
			return left["method"].(string) < right["method"].(string)
		}
		return left["path"].(string) < right["path"].(string)
	})

	routeStats := make([]gin.H, 0, len(globalMetrics.routeStats))
	routes := sortedRoutes(globalMetrics.routeStats)
	for _, route := range routes {
		stats := globalMetrics.routeStats[route]
		avgLatencyMs := 0.0
		if stats.Count > 0 {
			avgLatencyMs = float64(stats.TotalLatencyNanos) / float64(stats.Count) / float64(time.Millisecond)
		}
		routeStats = append(routeStats, gin.H{
			"method":          route.Method,
			"path":            route.Path,
			"count":           stats.Count,
			"error_count":     stats.ErrorCount,
			"avg_latency_ms":  avgLatencyMs,
			"latency_buckets": latencyBucketsSnapshot(stats.LatencyBuckets, stats.Count),
		})
	}

	avgLatencyMs := 0.0
	if totalRequests > 0 {
		avgLatencyMs = float64(totalLatencyNanos) / float64(totalRequests) / float64(time.Millisecond)
	}

	return gin.H{
		"started_at":     globalMetrics.startedAt.Format(time.RFC3339),
		"uptime_seconds": int64(time.Since(globalMetrics.startedAt).Seconds()),
		"total_requests": totalRequests,
		"in_flight":      inFlight,
		"avg_latency_ms": avgLatencyMs,
		"error_count":    errorCount,
		"latency_buckets": gin.H{
			"unit":    "milliseconds",
			"buckets": latencyBucketsSnapshot(latencyBuckets, totalRequests),
		},
		"status_counts":       statusCounts,
		"route_counts":        routeCounts,
		"route_status_counts": routeStatusCounts,
		"route_stats":         routeStats,
		"runtime":             runtimeSnapshot(),
		"database_pool":       databasePoolSnapshot(),
	}
}

func PrometheusMetrics() string {
	totalRequests := globalMetrics.totalRequests.Load()
	inFlight := globalMetrics.inFlight.Load()
	totalLatencyNanos := globalMetrics.totalLatencyNanos.Load()
	errorCount := globalMetrics.errorCount.Load()
	latencyBuckets := atomicLatencyBucketsSnapshot(globalMetrics.latencyBuckets)

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	var b strings.Builder
	b.WriteString("# HELP go_admin_kit_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE go_admin_kit_http_requests_total counter\n")
	fmt.Fprintf(&b, "go_admin_kit_http_requests_total %d\n", totalRequests)
	b.WriteString("# HELP go_admin_kit_http_in_flight_requests In-flight HTTP requests.\n")
	b.WriteString("# TYPE go_admin_kit_http_in_flight_requests gauge\n")
	fmt.Fprintf(&b, "go_admin_kit_http_in_flight_requests %d\n", inFlight)
	b.WriteString("# HELP go_admin_kit_http_request_errors_total Total HTTP requests ending with 5xx status or Gin errors.\n")
	b.WriteString("# TYPE go_admin_kit_http_request_errors_total counter\n")
	fmt.Fprintf(&b, "go_admin_kit_http_request_errors_total %d\n", errorCount)
	b.WriteString("# HELP go_admin_kit_http_request_duration_seconds HTTP request latency histogram.\n")
	b.WriteString("# TYPE go_admin_kit_http_request_duration_seconds histogram\n")
	for i, upperBound := range latencyBucketSeconds {
		fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_bucket{le=\"%s\"} %d\n", formatBucket(upperBound), latencyBuckets[i])
	}
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", totalRequests)
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_sum %.6f\n", float64(totalLatencyNanos)/float64(time.Second))
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_count %d\n", totalRequests)

	statuses := make([]int, 0, len(globalMetrics.statusCounts))
	for status := range globalMetrics.statusCounts {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	b.WriteString("# HELP go_admin_kit_http_responses_total Total HTTP responses by status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_responses_total counter\n")
	for _, status := range statuses {
		fmt.Fprintf(&b, "go_admin_kit_http_responses_total{status=\"%d\"} %d\n", status, globalMetrics.statusCounts[status])
	}

	routes := make([]routeMetric, 0, len(globalMetrics.routeCounts))
	for route := range globalMetrics.routeCounts {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	b.WriteString("# HELP go_admin_kit_http_route_requests_total Total HTTP requests by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_requests_total counter\n")
	for _, route := range routes {
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_requests_total{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			globalMetrics.routeCounts[route],
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_responses_total Total HTTP responses by method, route, and status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_responses_total counter\n")
	routeStatuses := make([]routeStatusMetric, 0, len(globalMetrics.routeStatusCounts))
	for routeStatus := range globalMetrics.routeStatusCounts {
		routeStatuses = append(routeStatuses, routeStatus)
	}
	sort.Slice(routeStatuses, func(i, j int) bool {
		if routeStatuses[i].Path == routeStatuses[j].Path {
			if routeStatuses[i].Method == routeStatuses[j].Method {
				return routeStatuses[i].Status < routeStatuses[j].Status
			}
			return routeStatuses[i].Method < routeStatuses[j].Method
		}
		return routeStatuses[i].Path < routeStatuses[j].Path
	})
	for _, routeStatus := range routeStatuses {
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_responses_total{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
			escapeLabel(routeStatus.Method),
			escapeLabel(routeStatus.Path),
			routeStatus.Status,
			globalMetrics.routeStatusCounts[routeStatus],
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_errors_total Total HTTP requests ending with 5xx status or Gin errors by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_errors_total counter\n")
	for _, route := range sortedRoutes(globalMetrics.routeStats) {
		stats := globalMetrics.routeStats[route]
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_errors_total{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			stats.ErrorCount,
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_request_duration_seconds HTTP request latency histogram by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_request_duration_seconds histogram\n")
	for _, route := range sortedRoutes(globalMetrics.routeStats) {
		stats := globalMetrics.routeStats[route]
		for i, upperBound := range latencyBucketSeconds {
			fmt.Fprintf(
				&b,
				"go_admin_kit_http_route_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"%s\"} %d\n",
				escapeLabel(route.Method),
				escapeLabel(route.Path),
				formatBucket(upperBound),
				stats.LatencyBuckets[i],
			)
		}
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"+Inf\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			stats.Count,
		)
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_sum{method=\"%s\",path=\"%s\"} %.6f\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			float64(stats.TotalLatencyNanos)/float64(time.Second),
		)
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_count{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			stats.Count,
		)
	}

	writeRuntimePrometheusMetrics(&b)
	writeDatabasePoolPrometheusMetrics(&b)
	return b.String()
}

func sortedRoutes(stats map[routeMetric]*routeStats) []routeMetric {
	routes := make([]routeMetric, 0, len(stats))
	for route := range stats {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func latencyBucketsSnapshot(counts []uint64, total uint64) []gin.H {
	buckets := make([]gin.H, 0, len(latencyBucketSeconds)+1)
	for i, upperBound := range latencyBucketSeconds {
		buckets = append(buckets, gin.H{
			"le_ms": int(math.Round(upperBound * 1000)),
			"count": counts[i],
		})
	}
	buckets = append(buckets, gin.H{
		"le_ms": "+Inf",
		"count": total,
	})
	return buckets
}

func atomicLatencyBucketsSnapshot(counts []atomic.Uint64) []uint64 {
	values := make([]uint64, len(counts))
	for i := range counts {
		values[i] = counts[i].Load()
	}
	return values
}

func runtimeSnapshot() gin.H {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return gin.H{
		"goroutines":       runtime.NumGoroutine(),
		"num_cpu":          runtime.NumCPU(),
		"gomaxprocs":       runtime.GOMAXPROCS(0),
		"alloc_bytes":      mem.Alloc,
		"heap_alloc_bytes": mem.HeapAlloc,
		"heap_inuse_bytes": mem.HeapInuse,
		"heap_objects":     mem.HeapObjects,
		"gc_count":         mem.NumGC,
	}
}

func databasePoolSnapshot() gin.H {
	if database.DB == nil {
		return gin.H{"status": "not_initialized"}
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		return gin.H{"status": "error", "error": err.Error()}
	}
	stats := sqlDB.Stats()
	return gin.H{
		"status":               "ok",
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration_ms":     float64(stats.WaitDuration) / float64(time.Millisecond),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}

func writeRuntimePrometheusMetrics(b *strings.Builder) {
	snapshot := runtimeSnapshot()
	b.WriteString("# HELP go_admin_kit_go_goroutines Number of current goroutines.\n")
	b.WriteString("# TYPE go_admin_kit_go_goroutines gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_goroutines %d\n", snapshot["goroutines"])
	b.WriteString("# HELP go_admin_kit_go_memory_alloc_bytes Bytes of allocated heap objects.\n")
	b.WriteString("# TYPE go_admin_kit_go_memory_alloc_bytes gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_memory_alloc_bytes %d\n", snapshot["alloc_bytes"])
	b.WriteString("# HELP go_admin_kit_go_heap_objects Number of allocated heap objects.\n")
	b.WriteString("# TYPE go_admin_kit_go_heap_objects gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_heap_objects %d\n", snapshot["heap_objects"])
	b.WriteString("# HELP go_admin_kit_go_gc_total Completed GC cycles.\n")
	b.WriteString("# TYPE go_admin_kit_go_gc_total counter\n")
	fmt.Fprintf(b, "go_admin_kit_go_gc_total %d\n", snapshot["gc_count"])
}

func writeDatabasePoolPrometheusMetrics(b *strings.Builder) {
	if database.DB == nil {
		return
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		return
	}
	stats := sqlDB.Stats()
	b.WriteString("# HELP go_admin_kit_db_open_connections Open database connections.\n")
	b.WriteString("# TYPE go_admin_kit_db_open_connections gauge\n")
	fmt.Fprintf(b, "go_admin_kit_db_open_connections %d\n", stats.OpenConnections)
	b.WriteString("# HELP go_admin_kit_db_in_use_connections Database connections currently in use.\n")
	b.WriteString("# TYPE go_admin_kit_db_in_use_connections gauge\n")
	fmt.Fprintf(b, "go_admin_kit_db_in_use_connections %d\n", stats.InUse)
	b.WriteString("# HELP go_admin_kit_db_idle_connections Idle database connections.\n")
	b.WriteString("# TYPE go_admin_kit_db_idle_connections gauge\n")
	fmt.Fprintf(b, "go_admin_kit_db_idle_connections %d\n", stats.Idle)
	b.WriteString("# HELP go_admin_kit_db_wait_total Total waits for a database connection.\n")
	b.WriteString("# TYPE go_admin_kit_db_wait_total counter\n")
	fmt.Fprintf(b, "go_admin_kit_db_wait_total %d\n", stats.WaitCount)
	b.WriteString("# HELP go_admin_kit_db_wait_duration_seconds Total time blocked waiting for a database connection.\n")
	b.WriteString("# TYPE go_admin_kit_db_wait_duration_seconds counter\n")
	fmt.Fprintf(b, "go_admin_kit_db_wait_duration_seconds %.6f\n", stats.WaitDuration.Seconds())
}

func formatBucket(bucket float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", bucket), "0"), ".")
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
