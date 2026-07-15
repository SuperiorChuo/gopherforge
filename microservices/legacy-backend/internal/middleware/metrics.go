package middleware

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
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
	Count             atomic.Uint64
	ErrorCount        atomic.Uint64
	TotalLatencyNanos atomic.Int64
	LatencyBuckets    []atomic.Uint64
}

type metricsStore struct {
	startedAt         time.Time
	totalRequests     atomic.Uint64
	inFlight          atomic.Int64
	totalLatencyNanos atomic.Int64
	errorCount        atomic.Uint64
	statusCounts      sync.Map
	routeCounts       sync.Map
	routeStatusCounts sync.Map
	latencyBuckets    []atomic.Uint64
	routeStats        sync.Map
}

var latencyBucketSeconds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var globalMetrics = newMetricsStore()

// DatabasePoolStatsProvider provides database connection pool stats for metrics output.
type DatabasePoolStatsProvider interface {
	DatabaseStats() (sql.DBStats, error)
}

var (
	errDatabasePoolNotInitialized = errors.New("database not initialized")
	databasePoolStatsProviderMu   sync.RWMutex
	databasePoolStatsProvider     DatabasePoolStatsProvider = defaultDatabasePoolStatsProvider{}
)

func newMetricsStore() *metricsStore {
	return &metricsStore{
		startedAt:      time.Now(),
		latencyBuckets: make([]atomic.Uint64, len(latencyBucketSeconds)),
	}
}

// SetMetricsDatabasePoolStatsProvider replaces the database stats provider and returns a restore function.
func SetMetricsDatabasePoolStatsProvider(provider DatabasePoolStatsProvider) func() {
	databasePoolStatsProviderMu.Lock()
	previous := databasePoolStatsProvider
	if provider == nil {
		databasePoolStatsProvider = defaultDatabasePoolStatsProvider{}
	} else {
		databasePoolStatsProvider = provider
	}
	databasePoolStatsProviderMu.Unlock()

	return func() {
		databasePoolStatsProviderMu.Lock()
		databasePoolStatsProvider = previous
		databasePoolStatsProviderMu.Unlock()
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

		metricCounter(&globalMetrics.statusCounts, status).Add(1)
		metricCounter(&globalMetrics.routeCounts, metric).Add(1)
		metricCounter(&globalMetrics.routeStatusCounts, statusMetric).Add(1)
		stats := metricRouteStats(&globalMetrics.routeStats, metric)
		stats.Count.Add(1)
		if isError {
			stats.ErrorCount.Add(1)
		}
		stats.TotalLatencyNanos.Add(latency.Nanoseconds())
		recordAtomicLatencyBucket(stats.LatencyBuckets, latency)
	}
}

const httpStatusInternalServerError = 500

func (m *metricsStore) recordLatency(latency time.Duration) {
	recordAtomicLatencyBucket(m.latencyBuckets, latency)
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

	statusCounts := snapshotStatusCounts(&globalMetrics.statusCounts)

	routeCounts := snapshotRouteCounts(&globalMetrics.routeCounts)

	routeStatuses := sortedRouteStatusMetrics(&globalMetrics.routeStatusCounts)
	routeStatusCounts := make([]gin.H, 0, len(routeStatuses))
	for _, route := range routeStatuses {
		count := loadMetricCounter(&globalMetrics.routeStatusCounts, route)
		routeStatusCounts = append(routeStatusCounts, gin.H{
			"method": route.Method,
			"path":   route.Path,
			"status": route.Status,
			"count":  count,
		})
	}

	routes := sortedRoutes(&globalMetrics.routeStats)
	routeStats := make([]gin.H, 0, len(routes))
	for _, route := range routes {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		count := stats.Count.Load()
		totalRouteLatencyNanos := stats.TotalLatencyNanos.Load()
		avgLatencyMs := 0.0
		if count > 0 {
			avgLatencyMs = float64(totalRouteLatencyNanos) / float64(count) / float64(time.Millisecond)
		}
		routeStats = append(routeStats, gin.H{
			"method":          route.Method,
			"path":            route.Path,
			"count":           count,
			"error_count":     stats.ErrorCount.Load(),
			"avg_latency_ms":  avgLatencyMs,
			"latency_buckets": latencyBucketsSnapshot(atomicLatencyBucketsSnapshot(stats.LatencyBuckets), count),
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

	statuses := sortedStatuses(&globalMetrics.statusCounts)
	b.WriteString("# HELP go_admin_kit_http_responses_total Total HTTP responses by status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_responses_total counter\n")
	for _, status := range statuses {
		fmt.Fprintf(&b, "go_admin_kit_http_responses_total{status=\"%d\"} %d\n", status, loadMetricCounter(&globalMetrics.statusCounts, status))
	}

	routes := sortedRouteMetrics(&globalMetrics.routeCounts)
	b.WriteString("# HELP go_admin_kit_http_route_requests_total Total HTTP requests by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_requests_total counter\n")
	for _, route := range routes {
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_requests_total{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			loadMetricCounter(&globalMetrics.routeCounts, route),
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_responses_total Total HTTP responses by method, route, and status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_responses_total counter\n")
	routeStatuses := sortedRouteStatusMetrics(&globalMetrics.routeStatusCounts)
	for _, routeStatus := range routeStatuses {
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_responses_total{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
			escapeLabel(routeStatus.Method),
			escapeLabel(routeStatus.Path),
			routeStatus.Status,
			loadMetricCounter(&globalMetrics.routeStatusCounts, routeStatus),
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_errors_total Total HTTP requests ending with 5xx status or Gin errors by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_errors_total counter\n")
	for _, route := range sortedRoutes(&globalMetrics.routeStats) {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_errors_total{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			stats.ErrorCount.Load(),
		)
	}
	b.WriteString("# HELP go_admin_kit_http_route_request_duration_seconds HTTP request latency histogram by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_request_duration_seconds histogram\n")
	for _, route := range sortedRoutes(&globalMetrics.routeStats) {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		buckets := atomicLatencyBucketsSnapshot(stats.LatencyBuckets)
		count := stats.Count.Load()
		for i, upperBound := range latencyBucketSeconds {
			fmt.Fprintf(
				&b,
				"go_admin_kit_http_route_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"%s\"} %d\n",
				escapeLabel(route.Method),
				escapeLabel(route.Path),
				formatBucket(upperBound),
				buckets[i],
			)
		}
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",le=\"+Inf\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			count,
		)
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_sum{method=\"%s\",path=\"%s\"} %.6f\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			float64(stats.TotalLatencyNanos.Load())/float64(time.Second),
		)
		fmt.Fprintf(
			&b,
			"go_admin_kit_http_route_request_duration_seconds_count{method=\"%s\",path=\"%s\"} %d\n",
			escapeLabel(route.Method),
			escapeLabel(route.Path),
			count,
		)
	}

	writeRuntimePrometheusMetrics(&b)
	writeDatabasePoolPrometheusMetrics(&b)
	return b.String()
}

func metricCounter(counters *sync.Map, key any) *atomic.Uint64 {
	counter := &atomic.Uint64{}
	actual, _ := counters.LoadOrStore(key, counter)
	return actual.(*atomic.Uint64)
}

func metricRouteStats(stats *sync.Map, route routeMetric) *routeStats {
	candidate := &routeStats{LatencyBuckets: make([]atomic.Uint64, len(latencyBucketSeconds))}
	actual, _ := stats.LoadOrStore(route, candidate)
	return actual.(*routeStats)
}

func loadMetricCounter(counters *sync.Map, key any) uint64 {
	value, ok := counters.Load(key)
	if !ok {
		return 0
	}
	return value.(*atomic.Uint64).Load()
}

func loadRouteStats(stats *sync.Map, route routeMetric) *routeStats {
	value, ok := stats.Load(route)
	if !ok {
		return nil
	}
	return value.(*routeStats)
}

func snapshotStatusCounts(counts *sync.Map) map[int]uint64 {
	statusCounts := make(map[int]uint64)
	counts.Range(func(key, value any) bool {
		statusCounts[key.(int)] = value.(*atomic.Uint64).Load()
		return true
	})
	return statusCounts
}

func snapshotRouteCounts(counts *sync.Map) map[string]uint64 {
	routeCounts := make(map[string]uint64)
	counts.Range(func(key, value any) bool {
		route := key.(routeMetric)
		routeCounts[route.Method+" "+route.Path] = value.(*atomic.Uint64).Load()
		return true
	})
	return routeCounts
}

func sortedStatuses(counts *sync.Map) []int {
	statuses := make([]int, 0)
	counts.Range(func(key, _ any) bool {
		statuses = append(statuses, key.(int))
		return true
	})
	sort.Ints(statuses)
	return statuses
}

func sortedRouteMetrics(counts *sync.Map) []routeMetric {
	routes := make([]routeMetric, 0)
	counts.Range(func(key, _ any) bool {
		routes = append(routes, key.(routeMetric))
		return true
	})
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func sortedRouteStatusMetrics(counts *sync.Map) []routeStatusMetric {
	routes := make([]routeStatusMetric, 0)
	counts.Range(func(key, _ any) bool {
		routes = append(routes, key.(routeStatusMetric))
		return true
	})
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			if routes[i].Method == routes[j].Method {
				return routes[i].Status < routes[j].Status
			}
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func sortedRoutes(stats *sync.Map) []routeMetric {
	return sortedRouteMetrics(stats)
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
	stats, err := currentDatabasePoolStatsProvider().DatabaseStats()
	if errors.Is(err, errDatabasePoolNotInitialized) {
		return gin.H{"status": "not_initialized"}
	}
	if err != nil {
		return gin.H{"status": "error", "error": err.Error()}
	}
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
	stats, err := currentDatabasePoolStatsProvider().DatabaseStats()
	if err != nil {
		return
	}
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

func currentDatabasePoolStatsProvider() DatabasePoolStatsProvider {
	databasePoolStatsProviderMu.RLock()
	provider := databasePoolStatsProvider
	databasePoolStatsProviderMu.RUnlock()
	if provider == nil {
		return defaultDatabasePoolStatsProvider{}
	}
	return provider
}

type defaultDatabasePoolStatsProvider struct{}

func (defaultDatabasePoolStatsProvider) DatabaseStats() (sql.DBStats, error) {
	return sql.DBStats{}, errDatabasePoolNotInitialized
}

func formatBucket(bucket float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", bucket), "0"), ".")
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
