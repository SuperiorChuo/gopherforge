package authz

import (
	"context"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

// goRedisRemoteCache 是 RemoteCache 的 go-redis 通用实现（authz 收敛批次 1）：
// 各服务 internal/pkg/redis 拷贝同构（已验证仅 config 路径差异），服务侧装配时
// 一行注入：authz.SetRemoteCache(authz.NewGoRedisRemoteCache(redisstore.Client))。
type goRedisRemoteCache struct {
	client *redis.Client
}

// NewGoRedisRemoteCache 用现成的 *redis.Client 构造 RemoteCache。
func NewGoRedisRemoteCache(client *redis.Client) RemoteCache {
	return &goRedisRemoteCache{client: client}
}

func (c *goRedisRemoteCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}

func (c *goRedisRemoteCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *goRedisRemoteCache) Del(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *goRedisRemoteCache) PublishString(ctx context.Context, channel, payload string) error {
	return c.client.Publish(ctx, channel, payload).Err()
}

// StartSubscriber 订阅 channel 并派发 handler，返回的 io.Closer 停止订阅。
// 与各服务原 internal/pkg/redis.StartSubscriber 语义一致。
func (c *goRedisRemoteCache) StartSubscriber(ctx context.Context, channel string, handler func(context.Context, string)) (io.Closer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	subCtx, cancel := context.WithCancel(ctx)
	pubsub := c.client.Subscribe(subCtx, channel)
	if _, err := pubsub.Receive(subCtx); err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		receiveChannel := pubsub.Channel()
		for {
			select {
			case <-subCtx.Done():
				return
			case message, ok := <-receiveChannel:
				if !ok {
					return
				}
				if handler != nil {
					handler(subCtx, message.Payload)
				}
			}
		}
	}()

	return closerFunc(func() error {
		cancel()
		err := pubsub.Close()
		<-done
		return err
	}), nil
}

// closerFunc 把闭包适配成 io.Closer。
type closerFunc func() error

func (f closerFunc) Close() error { return f() }
