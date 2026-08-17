package cache

import (
	"context"
	"fmt"
	"testing"
)

func TestDictCodeCacheRoundTripsEntries(t *testing.T) {
	store := setupCacheTestRedis(t)
	service := NewCacheService()
	ctx := context.Background()

	entries := map[string]DictEntry{
		"gender": {Found: true, Items: []DictItem{{ID: 1, DictTypeID: 7, Label: "男", Value: "1"}}},
		// 已知不存在的编码也会被缓存，这样错误的编码就不会每次请求都查询数据库。
		"missing": {Found: false},
	}
	if err := service.SetDictCodesContext(ctx, 1, entries); err != nil {
		t.Fatalf("SetDictCodesContext(): %v", err)
	}
	if !store.Exists(fmt.Sprintf(KeyDictData, uint(1), "gender")) {
		t.Fatal("gender entry was not written to redis")
	}

	hits := service.GetDictCodesContext(ctx, 1, []string{"gender", "missing", "never-written"})
	if len(hits) != 2 {
		t.Fatalf("GetDictCodesContext() = %+v, want two hits (unwritten code must miss)", hits)
	}
	gender := hits["gender"]
	if !gender.Found || len(gender.Items) != 1 || gender.Items[0].Label != "男" {
		t.Fatalf("GetDictCodesContext()[gender] = %+v, want the stored item", gender)
	}
	if absent, ok := hits["missing"]; !ok || absent.Found {
		t.Fatalf("GetDictCodesContext()[missing] = %+v, want a cached not-found answer", absent)
	}
	if _, ok := hits["never-written"]; ok {
		t.Fatal("an unwritten code must not report a cache hit")
	}

	entry, ok := service.GetDictCodeContext(ctx, 1, "gender")
	if !ok || !entry.Found || len(entry.Items) != 1 {
		t.Fatalf("GetDictCodeContext() = %+v ok=%v, want the stored entry", entry, ok)
	}
}

func TestDictCacheIsolatesTenants(t *testing.T) {
	setupCacheTestRedis(t)
	service := NewCacheService()
	ctx := context.Background()

	if err := service.SetDictCodesContext(ctx, 1, map[string]DictEntry{
		"gender": {Found: true, Items: []DictItem{{ID: 1, Label: "tenant-one"}}},
	}); err != nil {
		t.Fatalf("SetDictCodesContext(tenant 1): %v", err)
	}
	if err := service.SetDictCodesContext(ctx, 2, map[string]DictEntry{
		"gender": {Found: true, Items: []DictItem{{ID: 2, Label: "tenant-two"}}},
	}); err != nil {
		t.Fatalf("SetDictCodesContext(tenant 2): %v", err)
	}

	one := service.GetDictCodesContext(ctx, 1, []string{"gender"})
	two := service.GetDictCodesContext(ctx, 2, []string{"gender"})
	if one["gender"].Items[0].Label != "tenant-one" {
		t.Fatalf("tenant 1 read = %+v, want tenant-one", one["gender"])
	}
	if two["gender"].Items[0].Label != "tenant-two" {
		t.Fatalf("tenant 2 read = %+v, want tenant-two", two["gender"])
	}

	// 没有任何缓存数据的租户必须未命中，而不能借用其他租户的数据。
	if hits := service.GetDictCodesContext(ctx, 3, []string{"gender"}); len(hits) != 0 {
		t.Fatalf("tenant 3 read = %+v, want a miss", hits)
	}
}

func TestDictAllCacheRoundTripsPerTenant(t *testing.T) {
	setupCacheTestRedis(t)
	service := NewCacheService()
	ctx := context.Background()

	data := map[string][]DictItem{
		"gender": {{ID: 1, Label: "男", Value: "1"}},
		"status": {{ID: 2, Label: "启用", Value: "1"}},
	}
	if err := service.SetAllDictDataContext(ctx, 1, data); err != nil {
		t.Fatalf("SetAllDictDataContext(): %v", err)
	}

	cached, ok := service.GetAllDictDataContext(ctx, 1)
	if !ok || len(cached) != 2 || len(cached["gender"]) != 1 {
		t.Fatalf("GetAllDictDataContext() = %+v ok=%v, want the stored map", cached, ok)
	}
	if _, ok := service.GetAllDictDataContext(ctx, 2); ok {
		t.Fatal("tenant 2 must not see tenant 1's /dicts/all payload")
	}
}

func TestDelAllDictDataContextClearsEveryTenantAndKind(t *testing.T) {
	store := setupCacheTestRedis(t)
	service := NewCacheService()
	ctx := context.Background()

	if err := service.SetDictCodesContext(ctx, 1, map[string]DictEntry{
		"gender": {Found: true, Items: []DictItem{{ID: 1}}},
	}); err != nil {
		t.Fatalf("SetDictCodesContext(tenant 1): %v", err)
	}
	if err := service.SetDictCodesContext(ctx, 2, map[string]DictEntry{
		"gender": {Found: true, Items: []DictItem{{ID: 2}}},
	}); err != nil {
		t.Fatalf("SetDictCodesContext(tenant 2): %v", err)
	}
	if err := service.SetAllDictDataContext(ctx, 1, map[string][]DictItem{"gender": {{ID: 1}}}); err != nil {
		t.Fatalf("SetAllDictDataContext(): %v", err)
	}

	if err := service.DelAllDictDataContext(ctx); err != nil {
		t.Fatalf("DelAllDictDataContext(): %v", err)
	}

	// 一次写入不能留下任何残留：两个租户各自的编码条目、
	// /dicts/all 数据块，以及索引本身。
	for _, key := range []string{
		fmt.Sprintf(KeyDictData, uint(1), "gender"),
		fmt.Sprintf(KeyDictData, uint(2), "gender"),
		fmt.Sprintf(KeyDictAll, uint(1)),
		KeyDictIndex,
	} {
		if store.Exists(key) {
			t.Fatalf("key %q survived invalidation", key)
		}
	}
	if hits := service.GetDictCodesContext(ctx, 1, []string{"gender"}); len(hits) != 0 {
		t.Fatalf("GetDictCodesContext() after invalidation = %+v, want empty", hits)
	}
	if _, ok := service.GetAllDictDataContext(ctx, 1); ok {
		t.Fatal("GetAllDictDataContext() after invalidation should miss")
	}
}

func TestDictCacheDisabledByZeroTTL(t *testing.T) {
	setupCacheTestRedis(t)
	t.Setenv(EnvDictCacheTTL, "0")
	service := NewCacheService()
	ctx := context.Background()

	if DictCacheEnabled() {
		t.Fatal("DictCacheEnabled() should be false at TTL 0")
	}
	if err := service.SetDictCodesContext(ctx, 1, map[string]DictEntry{"gender": {Found: true}}); err != nil {
		t.Fatalf("SetDictCodesContext(): %v", err)
	}
	if hits := service.GetDictCodesContext(ctx, 1, []string{"gender"}); len(hits) != 0 {
		t.Fatalf("GetDictCodesContext() = %+v, want no hits when disabled", hits)
	}
}

func TestDictDataExpireClampsConfiguredTTL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", DictDataDefaultExpire.String()},
		{"not-a-number", DictDataDefaultExpire.String()},
		{"-1", DictDataDefaultExpire.String()},
		{"60", "1m0s"},
		{"999999", dictDataMaxExpire.String()},
	}
	for _, tc := range cases {
		t.Setenv(EnvDictCacheTTL, tc.raw)
		if got := DictDataExpire().String(); got != tc.want {
			t.Fatalf("DictDataExpire() with %q = %s, want %s", tc.raw, got, tc.want)
		}
	}
}

func TestDictCacheDegradesWithoutRedis(t *testing.T) {
	// 未配置 Redis：读取必须未命中，写入必须报告缓存不可用，
	// 而不能在类型化 nil 的全局 client 上 panic。
	service := NewCacheServiceWithClient(nil)
	ctx := context.Background()

	if hits := service.GetDictCodesContext(ctx, 1, []string{"gender"}); hits != nil {
		t.Fatalf("GetDictCodesContext() = %+v, want nil without redis", hits)
	}
	if _, ok := service.GetAllDictDataContext(ctx, 1); ok {
		t.Fatal("GetAllDictDataContext() should miss without redis")
	}
	if err := service.DelAllDictDataContext(ctx); err != nil {
		t.Fatalf("DelAllDictDataContext() without redis = %v, want nil", err)
	}
}
