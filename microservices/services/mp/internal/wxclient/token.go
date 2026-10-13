package wxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"golang.org/x/sync/singleflight"
)

// defaultWxBaseURL 是微信公众平台 API 根地址。TokenManager/Client 都把它做成
// 字段以便测试注入 httptest.Server。
const defaultWxBaseURL = "https://api.weixin.qq.com"

// tokenExpiryGuard 是提前过期的余量：缓存 TTL = expires_in - 300s，防止拿到
// 一个即将失效的 token。
const tokenExpiryGuard = 300 * time.Second

// tokenCache 抽象 access_token 中控用到的 Redis 操作，便于单测用内存实现替换。
// redisx 的 *redis.Client 天然满足这些方法签名（见 RedisTokenCache 适配）。
type tokenCache interface {
	// Get 返回 key 的值；key 不存在返回空串 + nil（不视为错误）。
	Get(ctx context.Context, key string) (string, error)
	// Set 写入 key，ttl<=0 表示不过期。
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// SetNX 尝试加锁：成功（key 原先不存在）返回 true。
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	// Del 删除 key（幂等）。
	Del(ctx context.Context, key string) error
}

// SecretResolver 把账号解析成微信 appid + appsecret。token 中控只依赖它拿密钥，
// 不自己碰 env / system_settings（解耦，具体实现留给 M4/M5）。
type SecretResolver interface {
	ResolveSecret(ctx context.Context, account *store.MpAccount) (appID, secret string, err error)
}

// EnvSecretResolver 是最简默认实现：appid 用 account.AppID，appsecret 从名为
// account.SecretRef 的环境变量取。SecretRef 为空或对应 env 缺失则报错。
type EnvSecretResolver struct{}

func (EnvSecretResolver) ResolveSecret(_ context.Context, account *store.MpAccount) (string, string, error) {
	if account == nil {
		return "", "", fmt.Errorf("resolve secret: nil account")
	}
	if account.AppID == "" {
		return "", "", fmt.Errorf("resolve secret: empty app_id")
	}
	if account.SecretRef == "" {
		return "", "", fmt.Errorf("resolve secret: empty secret_ref for app_id %s", account.AppID)
	}
	secret := os.Getenv(account.SecretRef)
	if secret == "" {
		return "", "", fmt.Errorf("resolve secret: env %q is empty for app_id %s", account.SecretRef, account.AppID)
	}
	return account.AppID, secret, nil
}

// TokenManager 是 access_token 中控。设计目标：多实例只有一个去刷、同实例并发
// 只刷一次、拿到的 token 至少还有 5 分钟有效期。
type TokenManager struct {
	cache    tokenCache
	resolver SecretResolver
	http     *http.Client
	baseURL  string // 微信 API 根地址，默认 defaultWxBaseURL，测试可注入
	sf       singleflight.Group

	// lockWait 抢锁失败后回读缓存前的等待；默认 200ms，测试可调小。
	lockWait time.Duration
}

// TokenManagerOption 定制 TokenManager（主要给测试注入 baseURL / lockWait）。
type TokenManagerOption func(*TokenManager)

// WithTokenBaseURL 覆盖微信 API 根地址（测试指向 httptest.Server）。
func WithTokenBaseURL(base string) TokenManagerOption {
	return func(m *TokenManager) { m.baseURL = base }
}

// WithTokenHTTPClient 覆盖 http.Client。
func WithTokenHTTPClient(c *http.Client) TokenManagerOption {
	return func(m *TokenManager) { m.http = c }
}

// WithLockWait 覆盖抢锁失败后的回读等待时长。
func WithLockWait(d time.Duration) TokenManagerOption {
	return func(m *TokenManager) { m.lockWait = d }
}

// NewTokenManager 构造中控。cache 通常传 RedisTokenCache{Client}，resolver 传
// EnvSecretResolver{} 或后续的 system_settings 实现。
func NewTokenManager(cache tokenCache, resolver SecretResolver, opts ...TokenManagerOption) *TokenManager {
	m := &TokenManager{
		cache:    cache,
		resolver: resolver,
		http:     &http.Client{Timeout: 8 * time.Second},
		baseURL:  defaultWxBaseURL,
		lockWait: 200 * time.Millisecond,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

func tokenKey(appID string) string     { return "mp:token:" + appID }
func tokenLockKey(appID string) string { return "mp:token:lock:" + appID }

// GetToken 返回可用的 access_token。命中缓存直接返回；否则经 singleflight +
// 分布式锁刷新，保证跨实例只有一个真正打微信接口。
func (m *TokenManager) GetToken(ctx context.Context, account *store.MpAccount) (string, error) {
	if account == nil || account.AppID == "" {
		return "", fmt.Errorf("get token: nil or empty account")
	}
	appID := account.AppID

	// 快路径：缓存命中。
	if tok, err := m.cache.Get(ctx, tokenKey(appID)); err == nil && tok != "" {
		return tok, nil
	}

	// 慢路径：同实例并发合流到一次刷新。
	v, err, _ := m.sf.Do(appID, func() (any, error) {
		// 双检：等待期间别的 goroutine 可能已经填好缓存。
		if tok, err := m.cache.Get(ctx, tokenKey(appID)); err == nil && tok != "" {
			return tok, nil
		}

		// 分布式锁：多实例只允许一个刷新。SET NX PX 10000。
		locked, lerr := m.cache.SetNX(ctx, tokenLockKey(appID), "1", 10*time.Second)
		if lerr != nil {
			return "", fmt.Errorf("get token: acquire lock: %w", lerr)
		}
		if !locked {
			// 没抢到：让抢到的实例刷，稍等后回读缓存。
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(m.lockWait):
			}
			if tok, gerr := m.cache.Get(ctx, tokenKey(appID)); gerr == nil && tok != "" {
				return tok, nil
			}
			return "", fmt.Errorf("get token: lock held by another instance and cache still empty for app_id %s", appID)
		}
		defer func() { _ = m.cache.Del(ctx, tokenLockKey(appID)) }()

		return m.refresh(ctx, account)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// Invalidate 删除某 appid 的缓存 token，供失效重试（40001/42001）后强制重刷。
func (m *TokenManager) Invalidate(ctx context.Context, appID string) error {
	return m.cache.Del(ctx, tokenKey(appID))
}

// tokenResp 是 cgi-bin/token 的成功响应；失败时带 errcode/errmsg。
type tokenResp struct {
	wxCommonResp
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// refresh 真正打微信 cgi-bin/token 换新 token 并写缓存。调用者已持有分布式锁。
func (m *TokenManager) refresh(ctx context.Context, account *store.MpAccount) (string, error) {
	appID, secret, err := m.resolver.ResolveSecret(ctx, account)
	if err != nil {
		return "", fmt.Errorf("get token: resolve secret: %w", err)
	}

	q := url.Values{}
	q.Set("grant_type", "client_credential")
	q.Set("appid", appID)
	q.Set("secret", secret)
	reqURL := m.baseURL + "/cgi-bin/token?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return "", safeRequestError("get token", err)
	}
	defer resp.Body.Close()

	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("get token: decode (status %d): %w", resp.StatusCode, err)
	}
	if err := tr.asWxError(); err != nil {
		return "", err
	}
	if !successfulHTTPStatus(resp.StatusCode) {
		return "", unexpectedHTTPStatus("get token", resp.StatusCode)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("get token: empty access_token in response")
	}

	ttl := time.Duration(tr.ExpiresIn)*time.Second - tokenExpiryGuard
	if ttl <= 0 {
		// expires_in 异常小（<300s）时给个兜底短 TTL，避免负数/不过期。
		ttl = time.Duration(tr.ExpiresIn) * time.Second
		if ttl <= 0 {
			ttl = time.Minute
		}
	}
	if err := m.cache.Set(ctx, tokenKey(appID), tr.AccessToken, ttl); err != nil {
		// 缓存写失败不致命：本次仍返回 token，下次再刷。
		return tr.AccessToken, nil
	}
	return tr.AccessToken, nil
}
