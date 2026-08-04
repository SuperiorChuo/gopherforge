package monitor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"gorm.io/gorm"
)

type AlertMetricDefinition struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Unit        string   `json:"unit"`
	Description string   `json:"description"`
	Operators   []string `json:"operators"`
}

type AlertMetricCollector interface {
	CollectContext(ctx context.Context, metric string) (float64, error)
}

type alertMetricSource func(context.Context) (float64, error)

type DefaultAlertMetricCollector struct {
	sources map[string]alertMetricSource
}

var alertMetricDefinitions = []AlertMetricDefinition{
	{Key: "system.cpu.used_percent", Title: "CPU used", Unit: "percent", Description: "Host CPU utilization", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "system.memory.used_percent", Title: "Memory used", Unit: "percent", Description: "Host memory utilization", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "system.disk.used_percent", Title: "Disk used", Unit: "percent", Description: "Root filesystem utilization", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "postgres.connections.percent", Title: "PostgreSQL connections", Unit: "percent", Description: "Used PostgreSQL connections as a percentage of max_connections", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "redis.memory.used_bytes", Title: "Redis memory used", Unit: "bytes", Description: "Redis used_memory in bytes", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "redis.clients.connected", Title: "Redis connected clients", Unit: "count", Description: "Connected Redis client count", Operators: []string{"gt", "gte", "lt", "lte"}},
	{Key: "pg.slow_queries", Title: "PostgreSQL slow queries", Unit: "count", Description: "Active queries running longer than 10 seconds", Operators: []string{"gt", "gte"}},
	{Key: "redis.keys_evicted_per_sec", Title: "Redis keys evicted", Unit: "count/s", Description: "Evicted keys per second since last evaluation", Operators: []string{"gt", "gte"}},
	{Key: "redis.keys_expired_per_sec", Title: "Redis keys expired", Unit: "count/s", Description: "Expired keys per second since last evaluation", Operators: []string{"gt", "gte"}},
	{Key: "system.disk.read_bytes_per_sec", Title: "Disk read throughput", Unit: "B/s", Description: "Aggregated disk read bytes per second", Operators: []string{"gt", "gte"}},
	{Key: "system.disk.write_bytes_per_sec", Title: "Disk write throughput", Unit: "B/s", Description: "Aggregated disk write bytes per second", Operators: []string{"gt", "gte"}},
	{Key: "system.net.bytes_recv_per_sec", Title: "Network receive throughput", Unit: "B/s", Description: "Aggregated network receive bytes per second", Operators: []string{"gt", "gte"}},
	{Key: "system.net.bytes_sent_per_sec", Title: "Network send throughput", Unit: "B/s", Description: "Aggregated network send bytes per second", Operators: []string{"gt", "gte"}},
}

func ListAlertMetrics() []AlertMetricDefinition {
	result := make([]AlertMetricDefinition, len(alertMetricDefinitions))
	for i, definition := range alertMetricDefinitions {
		result[i] = definition
		result[i].Operators = append([]string(nil), definition.Operators...)
	}
	return result
}

func NewDefaultAlertMetricCollector(db *gorm.DB, redisClient redis.UniversalClient) *DefaultAlertMetricCollector {
	collector := &DefaultAlertMetricCollector{sources: make(map[string]alertMetricSource)}
	collector.sources["system.cpu.used_percent"] = collectCPUUsedPercent
	collector.sources["system.memory.used_percent"] = collectMemoryUsedPercent
	collector.sources["system.disk.used_percent"] = collectDiskUsedPercent
	collector.sources["postgres.connections.percent"] = collectPostgresConnectionsPercent(db)
	collector.sources["redis.memory.used_bytes"] = collectRedisInfoValue(redisClient, "memory", "used_memory")
	collector.sources["redis.clients.connected"] = collectRedisInfoValue(redisClient, "clients", "connected_clients")
	collector.sources["pg.slow_queries"] = collectPostgresSlowQueries(db)
	// Counter metrics (cumulative INFO/gopsutil values) become per-second deltas.
	collector.sources["redis.keys_evicted_per_sec"] = newRateSource(collectRedisInfoValue(redisClient, "stats", "evicted_keys"))
	collector.sources["redis.keys_expired_per_sec"] = newRateSource(collectRedisInfoValue(redisClient, "stats", "expired_keys"))
	collector.sources["system.disk.read_bytes_per_sec"] = newRateSource(collectDiskIOBytes(true))
	collector.sources["system.disk.write_bytes_per_sec"] = newRateSource(collectDiskIOBytes(false))
	collector.sources["system.net.bytes_recv_per_sec"] = newRateSource(collectNetBytes(true))
	collector.sources["system.net.bytes_sent_per_sec"] = newRateSource(collectNetBytes(false))
	return collector
}

func (c *DefaultAlertMetricCollector) CollectContext(ctx context.Context, metric string) (float64, error) {
	if c == nil {
		return 0, errors.New("alert metric collector is not configured")
	}
	source, ok := c.sources[metric]
	if !ok {
		return 0, ErrInvalidAlertMetric
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return source(ctx)
}

func collectCPUUsedPercent(ctx context.Context) (float64, error) {
	values, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, errors.New("cpu utilization returned no values")
	}
	return values[0], nil
}

func collectMemoryUsedPercent(ctx context.Context) (float64, error) {
	value, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, err
	}
	return value.UsedPercent, nil
}

func collectDiskUsedPercent(ctx context.Context) (float64, error) {
	value, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return 0, err
	}
	return value.UsedPercent, nil
}

func collectPostgresConnectionsPercent(db *gorm.DB) alertMetricSource {
	return func(ctx context.Context) (float64, error) {
		if db == nil {
			return 0, errors.New("postgres database is not configured")
		}
		stats, err := monitordao.NewMySQLDAO(db).GetServerStatsContext(ctx)
		if err != nil {
			return 0, err
		}
		if stats.MaxConnections <= 0 {
			return 0, errors.New("postgres max_connections is not positive")
		}
		return float64(stats.Connections) / float64(stats.MaxConnections) * 100, nil
	}
}

func collectRedisInfoValue(client redis.UniversalClient, section, key string) alertMetricSource {
	return func(ctx context.Context) (float64, error) {
		if client == nil {
			return 0, errors.New("redis client is not configured")
		}
		payload, err := client.Info(ctx, section).Result()
		if err != nil {
			return 0, err
		}
		raw, exists := parseRedisInfo(payload)[key]
		if !exists || strings.TrimSpace(raw) == "" {
			return 0, fmt.Errorf("redis metric %s is missing", key)
		}
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("redis metric %s is invalid: %w", key, err)
		}
		if value < 0 {
			return 0, fmt.Errorf("redis metric %s is negative", key)
		}
		return float64(value), nil
	}
}

// collectPostgresSlowQueries counts active queries running longer than 10s —
// a dependency-free slow-query signal (no pg_stat_statements extension needed).
func collectPostgresSlowQueries(db *gorm.DB) alertMetricSource {
	return func(ctx context.Context) (float64, error) {
		if db == nil {
			return 0, errors.New("postgres database is not configured")
		}
		var count int64
		err := db.WithContext(ctx).Raw(
			"SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active' AND query_start < NOW() - INTERVAL '10 seconds'",
		).Scan(&count).Error
		if err != nil {
			return 0, err
		}
		return float64(count), nil
	}
}

func collectDiskIOBytes(read bool) alertMetricSource {
	return func(ctx context.Context) (float64, error) {
		counters, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return 0, err
		}
		var total uint64
		for _, c := range counters {
			if read {
				total += c.ReadBytes
			} else {
				total += c.WriteBytes
			}
		}
		return float64(total), nil
	}
}

func collectNetBytes(recv bool) alertMetricSource {
	return func(ctx context.Context) (float64, error) {
		counters, err := net.IOCountersWithContext(ctx, false)
		if err != nil {
			return 0, err
		}
		if len(counters) == 0 {
			return 0, errors.New("no network interfaces")
		}
		var total uint64
		for _, c := range counters {
			if recv {
				total += c.BytesRecv
			} else {
				total += c.BytesSent
			}
		}
		return float64(total), nil
	}
}

// newRateSource wraps a monotonic counter source and reports the per-second
// delta since the previous collection. The first call returns 0 (no baseline).
// Safe for concurrent use by the sampler and the alert evaluator.
func newRateSource(raw alertMetricSource) alertMetricSource {
	rate := &rateSource{raw: raw}
	return func(ctx context.Context) (float64, error) {
		return rate.value(ctx)
	}
}

type rateSource struct {
	mu     sync.Mutex
	raw    alertMetricSource
	prev   float64
	prevAt time.Time
	init   bool
}

func (r *rateSource) value(ctx context.Context) (float64, error) {
	value, err := r.raw(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.init {
		r.init = true
		r.prev = value
		r.prevAt = now
		return 0, nil
	}
	dt := now.Sub(r.prevAt).Seconds()
	rate := 0.0
	if dt > 0 && value >= r.prev {
		rate = (value - r.prev) / dt
	}
	r.prev = value
	r.prevAt = now
	return rate, nil
}

func isKnownAlertMetric(metric string) bool {
	for _, definition := range alertMetricDefinitions {
		if definition.Key == metric {
			return true
		}
	}
	return false
}

func alertMetricUnit(metric string) string {
	for _, definition := range alertMetricDefinitions {
		if definition.Key == metric {
			return definition.Unit
		}
	}
	return ""
}
