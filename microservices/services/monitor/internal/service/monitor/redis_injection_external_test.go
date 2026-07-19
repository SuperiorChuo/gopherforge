package monitor_test

import (
	"context"
	"testing"

	"github.com/go-admin-kit/server/internal/service/monitor"
	goredis "github.com/redis/go-redis/v9"
)

func TestNewRedisServiceWithClientIsUsableOutsidePackage(t *testing.T) {
	client := &stubRedisInfoClient{
		info: "redis_version:7.2.0\r\n" +
			"os:Linux\r\n" +
			"redis_mode:standalone\r\n" +
			"uptime_in_seconds:60\r\n" +
			"arch_bits:64\r\n" +
			"process_id:123\r\n" +
			"tcp_port:6379\r\n" +
			"used_memory:2048\r\n" +
			"used_memory_peak:4096\r\n" +
			"keyspace_hits:30\r\n" +
			"keyspace_misses:10\r\n" +
			"connected_clients:2\r\n",
		dbsize: 9,
		poolStats: &goredis.PoolStats{
			Hits:       4,
			Misses:     1,
			TotalConns: 2,
		},
	}

	service := monitor.NewRedisServiceWithClient(client)
	got, err := service.GetRedisInfoContext(context.Background())
	if err != nil {
		t.Fatalf("GetRedisInfoContext() error = %v", err)
	}

	stats := got["stats"].(map[string]any)
	if stats["keys"] != int64(9) {
		t.Fatalf("stats.keys = %v, want 9", stats["keys"])
	}
	if stats["hit_rate"] != "75.00%" {
		t.Fatalf("stats.hit_rate = %v, want 75.00%%", stats["hit_rate"])
	}
}

type stubRedisInfoClient struct {
	info      string
	infoErr   error
	dbsize    int64
	dbsizeErr error
	poolStats *goredis.PoolStats
}

func (s *stubRedisInfoClient) Info(ctx context.Context, section ...string) *goredis.StringCmd {
	return goredis.NewStringResult(s.info, s.infoErr)
}

func (s *stubRedisInfoClient) DBSize(ctx context.Context) *goredis.IntCmd {
	return goredis.NewIntResult(s.dbsize, s.dbsizeErr)
}

func (s *stubRedisInfoClient) PoolStats() *goredis.PoolStats {
	if s.poolStats == nil {
		return &goredis.PoolStats{}
	}
	return s.poolStats
}
