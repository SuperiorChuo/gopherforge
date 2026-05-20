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
