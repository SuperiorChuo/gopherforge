// Package metrics 提供零依赖（gin+stdlib）的进程内 Prometheus 文本指标：
// HTTP 请求计数/错误/延迟直方图（全局与按路由）+ Go runtime + 可选 DB 连接池。
// 与 monitor 服务 internal/middleware/metrics.go 同源精简，指标名同族
// （go_admin_kit_*），多服务靠 Prometheus 抓取配置的 service 标签区分。
//
// 用法（main.go 建 router 后、注册其余中间件前一行接入）：
//
//	metrics.Install(router)                 // 注册计量中间件 + GET /metrics
//	metrics.SetDBStats(sqlDB.Stats)         // 可选：接入连接池指标
//
// Install 先于 Logger/限流注册，/metrics 端点自身不进这些链。
// METRICS_ENABLED=false 时 Install 为空操作。
//
// 注意：构建上下文不含 shared 的服务（im/cc/crm/mp/notify/ticket/bpm/visibility）
// 各自持有 internal/metrics 同源副本（先例见 iploc），改动本文件须同步各副本。
package metrics

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Path 是指标端点路径；网关不路由该路径，仅供集群内 Prometheus 抓取。
const Path = "/metrics"

const httpStatusInternalServerError = 500

var latencyBucketSeconds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

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
	totalRequests     atomic.Uint64
	inFlight          atomic.Int64
	totalLatencyNanos atomic.Int64
	errorCount        atomic.Uint64
	statusCounts      sync.Map
	routeStats        sync.Map
	routeStatusCounts sync.Map
	latencyBuckets    []atomic.Uint64
}

var globalMetrics = &metricsStore{latencyBuckets: make([]atomic.Uint64, len(latencyBucketSeconds))}

var (
	dbStatsMu sync.RWMutex
	dbStatsFn func() sql.DBStats
)

// SetDBStats 接入数据库连接池指标（传 (*sql.DB).Stats）；不设置则输出中省略 db 段。
func SetDBStats(fn func() sql.DBStats) {
	dbStatsMu.Lock()
	dbStatsFn = fn
	dbStatsMu.Unlock()
}

// Enabled 读 METRICS_ENABLED（缺省与非法值視为 true）。
func Enabled() bool {
	raw := strings.TrimSpace(os.Getenv("METRICS_ENABLED"))
	if raw == "" {
		return true
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return enabled
}

// Install 注册计量中间件与 GET /metrics；METRICS_ENABLED=false 时不做任何事。
// 必须在其余路由/中间件注册之前调用（gin 路由只携带注册时刻已存在的中间件）。
func Install(r *gin.Engine) {
	if !Enabled() {
		return
	}
	r.GET(Path, func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(Render()))
	})
	r.Use(Middleware())
}

// Middleware 记录 HTTP 计数、错误与延迟直方图（全局与按路由）。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		globalMetrics.inFlight.Add(1)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			// 未命中路由（404 等）归并为一个桶，避免路径基数爆炸
			path = "unmatched"
		}
		metric := routeMetric{Method: c.Request.Method, Path: path}
		isError := status >= httpStatusInternalServerError || len(c.Errors) > 0

		globalMetrics.totalRequests.Add(1)
		globalMetrics.inFlight.Add(-1)
		globalMetrics.totalLatencyNanos.Add(latency.Nanoseconds())
		if isError {
			globalMetrics.errorCount.Add(1)
		}
		recordAtomicLatencyBucket(globalMetrics.latencyBuckets, latency)

		metricCounter(&globalMetrics.statusCounts, status).Add(1)
		metricCounter(&globalMetrics.routeStatusCounts, routeStatusMetric{Method: metric.Method, Path: metric.Path, Status: status}).Add(1)
		stats := metricRouteStats(&globalMetrics.routeStats, metric)
		stats.Count.Add(1)
		if isError {
			stats.ErrorCount.Add(1)
		}
		stats.TotalLatencyNanos.Add(latency.Nanoseconds())
		recordAtomicLatencyBucket(stats.LatencyBuckets, latency)
	}
}

// Render 输出 Prometheus 文本格式（0.0.4）。
func Render() string {
	totalRequests := globalMetrics.totalRequests.Load()
	totalLatencyNanos := globalMetrics.totalLatencyNanos.Load()
	latencyBuckets := atomicLatencyBucketsSnapshot(globalMetrics.latencyBuckets)

	var b strings.Builder
	b.WriteString("# HELP go_admin_kit_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE go_admin_kit_http_requests_total counter\n")
	fmt.Fprintf(&b, "go_admin_kit_http_requests_total %d\n", totalRequests)
	b.WriteString("# HELP go_admin_kit_http_in_flight_requests In-flight HTTP requests.\n")
	b.WriteString("# TYPE go_admin_kit_http_in_flight_requests gauge\n")
	fmt.Fprintf(&b, "go_admin_kit_http_in_flight_requests %d\n", globalMetrics.inFlight.Load())
	b.WriteString("# HELP go_admin_kit_http_request_errors_total Total HTTP requests ending with 5xx status or Gin errors.\n")
	b.WriteString("# TYPE go_admin_kit_http_request_errors_total counter\n")
	fmt.Fprintf(&b, "go_admin_kit_http_request_errors_total %d\n", globalMetrics.errorCount.Load())
	b.WriteString("# HELP go_admin_kit_http_request_duration_seconds HTTP request latency histogram.\n")
	b.WriteString("# TYPE go_admin_kit_http_request_duration_seconds histogram\n")
	for i, upperBound := range latencyBucketSeconds {
		fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_bucket{le=\"%s\"} %d\n", formatBucket(upperBound), latencyBuckets[i])
	}
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", totalRequests)
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_sum %.6f\n", float64(totalLatencyNanos)/float64(time.Second))
	fmt.Fprintf(&b, "go_admin_kit_http_request_duration_seconds_count %d\n", totalRequests)

	b.WriteString("# HELP go_admin_kit_http_responses_total Total HTTP responses by status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_responses_total counter\n")
	for _, status := range sortedStatuses(&globalMetrics.statusCounts) {
		fmt.Fprintf(&b, "go_admin_kit_http_responses_total{status=\"%d\"} %d\n", status, loadMetricCounter(&globalMetrics.statusCounts, status))
	}

	routes := sortedRoutes(&globalMetrics.routeStats)
	b.WriteString("# HELP go_admin_kit_http_route_requests_total Total HTTP requests by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_requests_total counter\n")
	for _, route := range routes {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		fmt.Fprintf(&b, "go_admin_kit_http_route_requests_total{method=%q,path=%q} %d\n", route.Method, route.Path, stats.Count.Load())
	}
	b.WriteString("# HELP go_admin_kit_http_route_responses_total Total HTTP responses by method, route, and status code.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_responses_total counter\n")
	for _, rs := range sortedRouteStatusMetrics(&globalMetrics.routeStatusCounts) {
		fmt.Fprintf(&b, "go_admin_kit_http_route_responses_total{method=%q,path=%q,status=\"%d\"} %d\n", rs.Method, rs.Path, rs.Status, loadMetricCounter(&globalMetrics.routeStatusCounts, rs))
	}
	b.WriteString("# HELP go_admin_kit_http_route_errors_total Total HTTP requests ending with 5xx status or Gin errors by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_errors_total counter\n")
	for _, route := range routes {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		fmt.Fprintf(&b, "go_admin_kit_http_route_errors_total{method=%q,path=%q} %d\n", route.Method, route.Path, stats.ErrorCount.Load())
	}
	b.WriteString("# HELP go_admin_kit_http_route_request_duration_seconds HTTP request latency histogram by method and route.\n")
	b.WriteString("# TYPE go_admin_kit_http_route_request_duration_seconds histogram\n")
	for _, route := range routes {
		stats := loadRouteStats(&globalMetrics.routeStats, route)
		if stats == nil {
			continue
		}
		buckets := atomicLatencyBucketsSnapshot(stats.LatencyBuckets)
		count := stats.Count.Load()
		for i, upperBound := range latencyBucketSeconds {
			fmt.Fprintf(&b, "go_admin_kit_http_route_request_duration_seconds_bucket{method=%q,path=%q,le=\"%s\"} %d\n", route.Method, route.Path, formatBucket(upperBound), buckets[i])
		}
		fmt.Fprintf(&b, "go_admin_kit_http_route_request_duration_seconds_bucket{method=%q,path=%q,le=\"+Inf\"} %d\n", route.Method, route.Path, count)
		fmt.Fprintf(&b, "go_admin_kit_http_route_request_duration_seconds_sum{method=%q,path=%q} %.6f\n", route.Method, route.Path, float64(stats.TotalLatencyNanos.Load())/float64(time.Second))
		fmt.Fprintf(&b, "go_admin_kit_http_route_request_duration_seconds_count{method=%q,path=%q} %d\n", route.Method, route.Path, count)
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

func recordAtomicLatencyBucket(buckets []atomic.Uint64, latency time.Duration) {
	seconds := latency.Seconds()
	for i, upperBound := range latencyBucketSeconds {
		if seconds <= upperBound {
			buckets[i].Add(1)
		}
	}
}

func atomicLatencyBucketsSnapshot(counts []atomic.Uint64) []uint64 {
	values := make([]uint64, len(counts))
	for i := range counts {
		values[i] = counts[i].Load()
	}
	return values
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

func sortedRoutes(stats *sync.Map) []routeMetric {
	routes := make([]routeMetric, 0)
	stats.Range(func(key, _ any) bool {
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

func writeRuntimePrometheusMetrics(b *strings.Builder) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	b.WriteString("# HELP go_admin_kit_go_goroutines Number of current goroutines.\n")
	b.WriteString("# TYPE go_admin_kit_go_goroutines gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_goroutines %d\n", runtime.NumGoroutine())
	b.WriteString("# HELP go_admin_kit_go_memory_alloc_bytes Bytes of allocated heap objects.\n")
	b.WriteString("# TYPE go_admin_kit_go_memory_alloc_bytes gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_memory_alloc_bytes %d\n", mem.Alloc)
	b.WriteString("# HELP go_admin_kit_go_heap_objects Number of allocated heap objects.\n")
	b.WriteString("# TYPE go_admin_kit_go_heap_objects gauge\n")
	fmt.Fprintf(b, "go_admin_kit_go_heap_objects %d\n", mem.HeapObjects)
	b.WriteString("# HELP go_admin_kit_go_gc_total Completed GC cycles.\n")
	b.WriteString("# TYPE go_admin_kit_go_gc_total counter\n")
	fmt.Fprintf(b, "go_admin_kit_go_gc_total %d\n", mem.NumGC)
}

func writeDatabasePoolPrometheusMetrics(b *strings.Builder) {
	dbStatsMu.RLock()
	fn := dbStatsFn
	dbStatsMu.RUnlock()
	if fn == nil {
		return
	}
	stats := fn()
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
