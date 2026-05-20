package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-admin-kit/server/internal/pkg/redis"
)

type RedisService struct{}

func NewRedisService() *RedisService {
	return &RedisService{}
}

// GetRedisInfo returns Redis information.
func (s *RedisService) GetRedisInfo() (map[string]any, error) {
	return s.GetRedisInfoContext(context.Background())
}

func (s *RedisService) GetRedisInfoContext(ctx context.Context) (map[string]any, error) {
	infoStr, err := redis.Client.Info(ctx).Result()
	if err != nil {
		return nil, err
	}

	info := parseRedisInfo(infoStr)

	// Load key counts.
	dbsize, _ := redis.Client.DBSize(ctx).Result()
	poolStats := redis.Client.PoolStats()
	keyspace := parseRedisKeyspace(info)

	// Build memory and runtime usage details.
	data := make(map[string]any)
	data["status"] = "ok"

	data["server"] = map[string]any{
		"version":        info["redis_version"],
		"os":             info["os"],
		"mode":           info["redis_mode"],
		"uptime":         info["uptime_in_seconds"],
		"uptime_seconds": parseInt64(info["uptime_in_seconds"]),
		"arch_bits":      info["arch_bits"],
		"process_id":     parseInt64(info["process_id"]),
		"tcp_port":       parseInt64(info["tcp_port"]),
	}

	data["memory"] = map[string]any{
		"used":          info["used_memory_human"],
		"peak":          info["used_memory_peak_human"],
		"lua":           info["used_memory_lua_human"],
		"fragmentation": info["mem_fragmentation_ratio"],
		"used_bytes":    parseInt64(info["used_memory"]),
		"peak_bytes":    parseInt64(info["used_memory_peak"]),
		"rss":           info["used_memory_rss_human"],
		"maxmemory":     firstNonEmpty(info["maxmemory_human"], formatBytes(parseInt64(info["maxmemory"]))),
		"mem_allocator": info["mem_allocator"],
		"dataset":       info["used_memory_dataset_human"],
		"overhead":      info["used_memory_overhead"],
	}

	data["stats"] = map[string]any{
		"connections":                info["connected_clients"],
		"ops":                        info["instantaneous_ops_per_sec"],
		"keys":                       dbsize,
		"hit_rate":                   calculateRedisHitRate(info),
		"total_connections_received": parseInt64(info["total_connections_received"]),
		"total_commands_processed":   parseInt64(info["total_commands_processed"]),
		"keyspace_hits":              parseInt64(info["keyspace_hits"]),
		"keyspace_misses":            parseInt64(info["keyspace_misses"]),
		"expired_keys":               parseInt64(info["expired_keys"]),
		"evicted_keys":               parseInt64(info["evicted_keys"]),
	}

	data["clients"] = map[string]any{
		"connected": parseInt64(info["connected_clients"]),
		"blocked":   parseInt64(info["blocked_clients"]),
		"tracking":  parseInt64(info["tracking_clients"]),
	}

	data["pool"] = map[string]any{
		"hits":        poolStats.Hits,
		"misses":      poolStats.Misses,
		"timeouts":    poolStats.Timeouts,
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
	}

	data["keyspace"] = map[string]any{
		"dbsize": dbsize,
		"dbs":    keyspace,
	}

	return data, nil
}

func parseRedisInfo(info string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			}
		}
	}
	return result
}

func calculateRedisHitRate(info map[string]string) string {
	hitsStr := info["keyspace_hits"]
	missesStr := info["keyspace_misses"]

	// Parse string counters from Redis INFO.
	var hits, misses float64
	fmt.Sscanf(hitsStr, "%f", &hits)
	fmt.Sscanf(missesStr, "%f", &misses)

	total := hits + misses
	if total == 0 {
		return "0%"
	}

	rate := (hits / total) * 100
	return fmt.Sprintf("%.2f%%", rate)
}

func parseRedisKeyspace(info map[string]string) map[string]map[string]int64 {
	result := make(map[string]map[string]int64)
	for key, value := range info {
		if !strings.HasPrefix(key, "db") {
			continue
		}

		dbInfo := make(map[string]int64)
		for _, part := range strings.Split(value, ",") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			dbInfo[kv[0]] = parseInt64(kv[1])
		}
		result[key] = dbInfo
	}
	return result
}
