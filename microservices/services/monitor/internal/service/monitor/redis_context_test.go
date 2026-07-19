package monitor

import (
	"context"
	"errors"
	"testing"

	internalredis "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestRedisServiceGetRedisInfoContextHonorsCanceledContext(t *testing.T) {
	oldClient := internalredis.Client
	internalredis.Client = goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() {
		_ = internalredis.Client.Close()
		internalredis.Client = oldClient
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&RedisService{}).GetRedisInfoContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetRedisInfoContext() error = %v, want context.Canceled", err)
	}
}

func TestRedisServiceGetRedisInfoContextUsesInjectedClient(t *testing.T) {
	oldClient := internalredis.Client
	internalredis.Client = nil
	t.Cleanup(func() {
		internalredis.Client = oldClient
	})

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
			"connected_clients:2\r\n" +
			"db0:keys=5,expires=1,avg_ttl=10\r\n",
		dbsize: 5,
		poolStats: &goredis.PoolStats{
			Hits:       10,
			Misses:     2,
			Timeouts:   1,
			TotalConns: 3,
			IdleConns:  1,
		},
	}

	got, err := NewRedisServiceWithClient(client).GetRedisInfoContext(context.Background())
	if err != nil {
		t.Fatalf("GetRedisInfoContext() error = %v", err)
	}

	if got["status"] != "ok" {
		t.Fatalf("status = %v, want ok", got["status"])
	}
	stats := got["stats"].(map[string]any)
	if stats["keys"] != int64(5) {
		t.Fatalf("stats.keys = %v, want 5", stats["keys"])
	}
	if stats["hit_rate"] != "75.00%" {
		t.Fatalf("stats.hit_rate = %v, want 75.00%%", stats["hit_rate"])
	}
	pool := got["pool"].(map[string]any)
	if pool["hits"] != uint32(10) {
		t.Fatalf("pool.hits = %v, want 10", pool["hits"])
	}
	if client.infoCalls != 1 || client.dbSizeCalls != 1 || client.poolStatsCalls != 1 {
		t.Fatalf("client calls = info:%d dbsize:%d pool:%d, want 1 each", client.infoCalls, client.dbSizeCalls, client.poolStatsCalls)
	}
}

type stubRedisInfoClient struct {
	info           string
	infoErr        error
	dbsize         int64
	dbsizeErr      error
	poolStats      *goredis.PoolStats
	infoCalls      int
	dbSizeCalls    int
	poolStatsCalls int
}

func (s *stubRedisInfoClient) Info(ctx context.Context, section ...string) *goredis.StringCmd {
	s.infoCalls++
	return goredis.NewStringResult(s.info, s.infoErr)
}

func (s *stubRedisInfoClient) DBSize(ctx context.Context) *goredis.IntCmd {
	s.dbSizeCalls++
	return goredis.NewIntResult(s.dbsize, s.dbsizeErr)
}

func (s *stubRedisInfoClient) PoolStats() *goredis.PoolStats {
	s.poolStatsCalls++
	if s.poolStats == nil {
		return &goredis.PoolStats{}
	}
	return s.poolStats
}
