// Package redisx 提供 mp-service 的 go-redis v9 客户端封装。access_token 中控
// （wxclient）依赖它做缓存 + 分布式锁。为避免和 config 包循环依赖，这里只收
// 原始参数（host/port/password/db），不 import config。
package redisx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config 是 redisx.New 的入参。DB 用 string 收（config.RedisDB 是 string），
// 内部解析成 int；解析失败回落到 0。
type Config struct {
	Host     string
	Port     string
	Password string
	DB       string
}

// New 构造 go-redis v9 客户端。不主动 Ping（Ping 走 Health），避免启动时
// Redis 未就绪就 panic——M2 约定 Redis 空/不通时降级不刷 token。
func New(cfg Config) *redis.Client {
	db := 0
	if v, err := strconv.Atoi(cfg.DB); err == nil {
		db = v
	}
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       db,
	})
}

// Health 在给定超时内 Ping 一次，供 /health 或启动自检调用。
func Health(ctx context.Context, c *redis.Client) error {
	if c == nil {
		return fmt.Errorf("redis client is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.Ping(ctx).Err()
}

// TokenCache 把 *redis.Client 适配成 access_token 中控所需的最小缓存/锁接口
// （wxclient.tokenCache）。方法签名按结构化接口对齐，wxclient 侧无需 import
// redisx，避免耦合方向反转。
type TokenCache struct {
	Client *redis.Client
}

// NewTokenCache 包装一个 redis 客户端。
func NewTokenCache(c *redis.Client) TokenCache { return TokenCache{Client: c} }

// Get 返回 key 的值；key 不存在返回空串 + nil（redis.Nil 不视为错误）。
func (t TokenCache) Get(ctx context.Context, key string) (string, error) {
	v, err := t.Client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// Set 写入 key；ttl<=0 表示不过期。
func (t TokenCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return t.Client.Set(ctx, key, value, ttl).Err()
}

// SetNX 尝试加锁，成功（key 原先不存在）返回 true。
func (t TokenCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return t.Client.SetNX(ctx, key, value, ttl).Result()
}

// Del 删除 key（幂等）。
func (t TokenCache) Del(ctx context.Context, key string) error {
	return t.Client.Del(ctx, key).Err()
}
