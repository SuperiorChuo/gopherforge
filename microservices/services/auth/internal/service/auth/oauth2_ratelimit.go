package auth

import (
	"context"
	"fmt"
	"time"

	localmodel "github.com/go-admin-kit/services/auth/internal/model"
	goredis "github.com/redis/go-redis/v9"
)

// OAuth2 协议端点的 per-client 配额。
//
// 落点是**认证之后**：配额按 client_id 计，而 client_id 只有在 client 认证
// 通过后才可信——认证前拿请求里的 client_id 计数，等于给了任何人耗尽他人
// 配额的手段。认证前的泛洪由服务级的按 IP 限流兜底（main.go 的
// DynamicRateLimit），两层各管一段。
//
// revoke 刻意不限流：吊销是安全止损动作，任何情况下都不该被自己的配额挡住。
const (
	// DefaultTokenRatePerMinute 是 client.TokenRatePerMinute 为 0 时的默认配额。
	// 按正常机器对接量给足余量，只拦异常调用。
	DefaultTokenRatePerMinute = 120
	tokenRateWindow           = time.Minute
	tokenRateKeyPrefix        = "oauth2:client_rate"
)

// tokenRateRedis 是限流只需要的两个命令。刻意不扩 cache.RedisClient——那会
// 迫使它的每个实现（含测试替身）都补齐这两个方法。
type tokenRateRedis interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	ExpireNX(ctx context.Context, key string, expiration time.Duration) *goredis.BoolCmd
}

// TokenRateLimitExceeded 由端点转成 HTTP 429。用 RFC 8628 注册过的
// slow_down 而不是自造错误码，客户端库能识别。
func tokenRateLimitExceeded(retryAfter time.Duration) *OAuth2Error {
	return &OAuth2Error{
		Code:        "slow_down",
		Description: "client exceeded the token endpoint rate limit",
		Status:      429,
		RetryAfter:  retryAfter,
	}
}

// EffectiveTokenRate 返回该 client 实际生效的每分钟配额。
func EffectiveTokenRate(client *localmodel.OAuth2Client) int {
	if client != nil && client.TokenRatePerMinute > 0 {
		return client.TokenRatePerMinute
	}
	return DefaultTokenRatePerMinute
}

// CheckTokenRate 对已认证 client 记一次调用并判断是否超额。
//
// Redis 不可用或命令报错时**放行**（与既有限流器同口径）：限流是防滥用而非
// 鉴权，缓存故障不该把正常的机器对接全打死。
func (s *OAuth2ServerService) CheckTokenRate(ctx context.Context, client *localmodel.OAuth2Client) *OAuth2Error {
	if client == nil {
		return nil
	}
	rdb := s.redis
	if rdb == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%s", tokenRateKeyPrefix, client.ClientID)
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return nil
	}
	// ExpireNX：只在本窗口首次计数时设过期，后续 INCR 不会把窗口往后推。
	if err := rdb.ExpireNX(ctx, key, tokenRateWindow).Err(); err != nil {
		return nil
	}
	if count > int64(EffectiveTokenRate(client)) {
		return tokenRateLimitExceeded(tokenRateWindow)
	}
	return nil
}
