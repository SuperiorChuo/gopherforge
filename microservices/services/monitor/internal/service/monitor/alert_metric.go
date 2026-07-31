package monitor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
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
