package redis

import (
	"context"
	"fmt"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
)

var (
	Client *redis.Client
)

// InitRedis initializes the Redis connection.
func InitRedis() error {
	cfg := config.Cfg.Redis

	Client = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx := context.Background()
	_, err := Client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect redis: %w", err)
	}

	logger.Info("redis connected",
		logger.String("address", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logger.Int("database", cfg.DB),
	)
	return nil
}
