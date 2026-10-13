package wxclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// memCache 是 tokenCache 的内存实现，供单测替换 Redis。带 TTL 过期语义但不起
// 后台清理——测试量小，Get 时惰性判过期即可。
type memCache struct {
	mu   sync.Mutex
	data map[string]memEntry
}

type memEntry struct {
	val string
	exp time.Time // 零值表示不过期
}

func newMemCache() *memCache { return &memCache{data: map[string]memEntry{}} }

func (m *memCache) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[key]
	if !ok {
		return "", nil
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		delete(m.data, key)
		return "", nil
	}
	return e.val, nil
}

func (m *memCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := memEntry{val: value}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	m.data[key] = e
	return nil
}

func (m *memCache) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.data[key]; ok {
		if e.exp.IsZero() || time.Now().Before(e.exp) {
			return false, nil
		}
	}
	e := memEntry{val: value}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	m.data[key] = e
	return true, nil
}

func (m *memCache) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// staticResolver 返回固定 appid/secret，绕过 env。
type staticResolver struct {
	appID  string
	secret string
	err    error
}

func (s staticResolver) ResolveSecret(_ context.Context, acc *store.MpAccount) (string, string, error) {
	if s.err != nil {
		return "", "", s.err
	}
	return s.appID, s.secret, nil
}

func testAccount() *store.MpAccount {
	return &store.MpAccount{AppID: "wxtestappid", SecretRef: "TEST_SECRET_REF"}
}

// TestGetTokenCachesAndSingleflights 验证：并发多次 GetToken 只打一次
// /cgi-bin/token（缓存 + singleflight + 分布式锁生效）。
func TestGetTokenCachesAndSingleflights(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/token" {
			atomic.AddInt64(&hits, 1)
			// 模拟一点网络延迟，放大并发竞态。
			time.Sleep(20 * time.Millisecond)
			fmt.Fprint(w, `{"access_token":"TOK-123","expires_in":7200}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cache := newMemCache()
	mgr := NewTokenManager(cache, staticResolver{appID: "wxtestappid", secret: "s3cr3t"},
		WithTokenBaseURL(srv.URL), WithLockWait(30*time.Millisecond))
	acc := testAccount()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	toks := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			toks[i], errs[i] = mgr.GetToken(context.Background(), acc)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("GetToken[%d] error: %v", i, err)
		}
		if toks[i] != "TOK-123" {
			t.Fatalf("GetToken[%d] = %q, want TOK-123", i, toks[i])
		}
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("token endpoint hit %d times, want exactly 1", got)
	}

	// 再来一次应命中缓存，仍是 1 次。
	if _, err := mgr.GetToken(context.Background(), acc); err != nil {
		t.Fatalf("cached GetToken error: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("after cached call token endpoint hit %d times, want 1", got)
	}
}

// TestGetTokenWxError 验证 errcode!=0 时返回 WxError 且不写缓存。
func TestGetTokenWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":40164,"errmsg":"invalid ip"}`)
	}))
	defer srv.Close()

	cache := newMemCache()
	mgr := NewTokenManager(cache, staticResolver{appID: "wxtestappid", secret: "s"},
		WithTokenBaseURL(srv.URL))

	_, err := mgr.GetToken(context.Background(), testAccount())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	we, ok := err.(*WxError)
	if !ok {
		t.Fatalf("expected *WxError, got %T: %v", err, err)
	}
	if we.Code != 40164 {
		t.Fatalf("WxError.Code = %d, want 40164", we.Code)
	}
	if v, _ := cache.Get(context.Background(), tokenKey("wxtestappid")); v != "" {
		t.Fatalf("cache should be empty on error, got %q", v)
	}
}

// TestInvalidate 验证 Invalidate 删除缓存后下次 GetToken 会重刷。
func TestInvalidate(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		fmt.Fprintf(w, `{"access_token":"TOK-%d","expires_in":7200}`, n)
	}))
	defer srv.Close()

	mgr := NewTokenManager(newMemCache(), staticResolver{appID: "wxtestappid", secret: "s"},
		WithTokenBaseURL(srv.URL))
	acc := testAccount()

	t1, err := mgr.GetToken(context.Background(), acc)
	if err != nil || t1 != "TOK-1" {
		t.Fatalf("first GetToken = %q, %v", t1, err)
	}
	if err := mgr.Invalidate(context.Background(), acc.AppID); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	t2, err := mgr.GetToken(context.Background(), acc)
	if err != nil || t2 != "TOK-2" {
		t.Fatalf("after invalidate GetToken = %q, %v (want TOK-2)", t2, err)
	}
}

// TestEnvSecretResolver 覆盖默认 env 解析实现的成功/失败路径。
func TestEnvSecretResolver(t *testing.T) {
	r := EnvSecretResolver{}

	// SecretRef 为空 → 报错。
	if _, _, err := r.ResolveSecret(context.Background(), &store.MpAccount{AppID: "a"}); err == nil {
		t.Fatal("expected error for empty secret_ref")
	}

	// env 存在 → 返回 appid + secret。
	t.Setenv("MP_TEST_SECRET", "topsecret")
	appID, secret, err := r.ResolveSecret(context.Background(), &store.MpAccount{AppID: "app1", SecretRef: "MP_TEST_SECRET"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if appID != "app1" || secret != "topsecret" {
		t.Fatalf("resolve = %q,%q want app1,topsecret", appID, secret)
	}
}
