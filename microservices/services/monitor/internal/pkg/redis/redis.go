package redis

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
)

var (
	Client *redis.Client
)

type StringSubscriber struct {
	cancel  context.CancelFunc
	channel string
	pubsub  *redis.PubSub
	once    sync.Once
	err     error
	done    chan struct{}
}

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

func PublishString(ctx context.Context, channel, payload string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if Client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return Client.Publish(ctx, channel, payload).Err()
}

func StartSubscriber(ctx context.Context, channel string, handler func(context.Context, string)) (*StringSubscriber, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if Client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	subCtx, cancel := context.WithCancel(ctx)
	pubsub := Client.Subscribe(subCtx, channel)
	if _, err := pubsub.Receive(subCtx); err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}

	subscriber := &StringSubscriber{
		cancel:  cancel,
		channel: channel,
		pubsub:  pubsub,
		done:    make(chan struct{}),
	}

	go func() {
		defer close(subscriber.done)
		receiveChannel := pubsub.Channel()
		for {
			select {
			case <-subCtx.Done():
				return
			case message, ok := <-receiveChannel:
				if !ok {
					if logger.Logger != nil && subCtx.Err() == nil {
						logger.Warn("redis subscriber channel closed", logger.String("channel", subscriber.channel))
					}
					return
				}
				if handler != nil {
					handler(subCtx, message.Payload)
				}
			}
		}
	}()

	return subscriber, nil
}

func (s *StringSubscriber) Close() error {
	if s == nil {
		return nil
	}

	s.once.Do(func() {
		s.cancel()
		s.err = s.pubsub.Close()
		<-s.done
	})

	return s.err
}

func Close() error {
	if Client == nil {
		return nil
	}

	err := Client.Close()
	Client = nil
	return err
}
