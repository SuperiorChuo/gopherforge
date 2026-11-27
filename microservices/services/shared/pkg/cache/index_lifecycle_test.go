package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func TestCacheIndexesExpireAfterTheirPayloads(t *testing.T) {
	cases := []struct {
		name, index string
		write       func(*CacheService) error
	}{
		{"permissions", KeyUserPermissionsIndex, func(s *CacheService) error { return s.SetUserPermissionsContext(t.Context(), 1, []string{"read"}) }},
		{"roles", KeyUserRolesIndex, func(s *CacheService) error { return s.SetUserRolesContext(t.Context(), 1, []string{"reader"}) }},
		{"permission headers", KeyUserPermHeaderIndex, func(s *CacheService) error { return s.SetUserPermHeaderContext(t.Context(), 1, "read") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := setupCacheTestRedis(t)
			setIndexExpiryMode(t, store.Addr(), "activate")
			if err := tc.write(NewCacheService()); err != nil {
				t.Fatal(err)
			}
			if ttl := store.TTL(tc.index); ttl <= time.Hour {
				t.Fatalf("index TTL=%v must outlive the 1h payload", ttl)
			}
			store.FastForward(3 * time.Hour)
			if store.Exists(tc.index) {
				t.Fatal("idle index survived expired payloads")
			}
		})
	}
}

func TestConsoleSessionIndexDoesNotAccumulateExpiredSessions(t *testing.T) {
	t.Setenv(EnvConsoleSessionCacheTTL, "30")
	store := setupCacheTestRedis(t)
	svc := NewCacheService()
	for i := 0; i < 100; i++ {
		store.FastForward(time.Minute)
		if err := svc.SetConsoleSessionContext(t.Context(), fmt.Sprintf("session-%d", i), ConsoleSessionIdentity{UserID: 42, TenantID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, index := range []string{KeyConsoleSessionIndex, fmt.Sprintf(KeyConsoleSessionUserIndex, 42)} {
		members, err := store.Members(index)
		if err != nil {
			t.Fatal(err)
		}
		if len(members) != 1 {
			t.Fatalf("%s retains %d members; only latest session is live", index, len(members))
		}
	}
	if _, ok := svc.GetConsoleSessionContext(t.Context(), "session-99"); !ok {
		t.Fatal("cleanup removed live session")
	}
}

func TestDictIndexDoesNotAccumulateExpiredEntries(t *testing.T) {
	t.Setenv(EnvDictCacheTTL, "30")
	store := setupCacheTestRedis(t)
	svc := NewCacheService()
	for i := 0; i < 100; i++ {
		store.FastForward(time.Minute)
		if err := svc.SetDictCodesContext(t.Context(), uint(i+1), map[string]DictEntry{"state": {Found: true}}); err != nil {
			t.Fatal(err)
		}
	}
	members, err := store.Members(KeyDictIndex)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("dictionary index retains %d members; want 1", len(members))
	}
}

func TestIndexCleanupPreservesLiveLegacyMembers(t *testing.T) {
	store := setupCacheTestRedis(t)
	setIndexExpiryMode(t, store.Addr(), "activate")
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	const legacy = "user:permheader:legacy"
	if err := client.Set(ctx, legacy, "legacy:read", 4*time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.SAdd(ctx, KeyUserPermHeaderIndex, legacy).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Expire(ctx, KeyUserPermHeaderIndex, 5*time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := client.SAdd(ctx, KeyUserPermHeaderIndex, fmt.Sprintf("user:permheader:expired-%d", i)).Err(); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewCacheServiceWithClient(client)
	for i := 0; i < 100; i++ {
		if err := svc.SetUserPermHeaderContext(ctx, 1, "read"); err != nil {
			t.Fatal(err)
		}
	}
	members, err := client.SMembers(ctx, KeyUserPermHeaderIndex).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("legacy index has %d members after sweep, want 2 live members", len(members))
	}
	if ttl := store.TTL(KeyUserPermHeaderIndex); ttl < 4*time.Hour {
		t.Fatalf("write shortened existing index TTL: %v", ttl)
	}
	if err := svc.DelAllUserPermHeadersContext(ctx); err != nil {
		t.Fatal(err)
	}
	if store.Exists(legacy) {
		t.Fatal("legacy live member escaped invalidation")
	}
}

func TestBatchedInvalidationDeletesAllIndexedEntries(t *testing.T) {
	store := setupCacheTestRedis(t)
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	pipe := client.TxPipeline()
	for i := 0; i < 1200; i++ {
		key := fmt.Sprintf(KeyUserPermissions, i+1)
		pipe.SAdd(ctx, key, "read")
		pipe.Expire(ctx, key, time.Hour)
		pipe.SAdd(ctx, KeyUserPermissionsIndex, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := NewCacheServiceWithClient(client).DelAllUserPermissionsContext(ctx); err != nil {
		t.Fatal(err)
	}
	if keys := store.Keys(); len(keys) != 0 {
		t.Fatalf("%d keys survived invalidation", len(keys))
	}
}

// Insert a new cache entry at the first Redis round-trip boundary during
// invalidation. The old SMEMBERS/snapshot + DEL-index sequence loses this entry.
func TestConcurrentCacheWriteCannotBecomeUnindexed(t *testing.T) {
	store := setupCacheTestRedis(t)
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	svc := NewCacheServiceWithClient(client)
	if err := svc.SetUserRolesContext(t.Context(), 1, []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	var injected atomic.Bool
	client.AddHook(&cacheIndexBoundaryHook{after: func() error {
		if injected.CompareAndSwap(false, true) {
			return svc.SetUserRolesContext(t.Context(), 2, []string{"reader"})
		}
		return nil
	}})
	if err := svc.DelAllUserRolesContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !injected.Load() {
		t.Fatal("concurrent write was not exercised")
	}
	key := fmt.Sprintf(KeyUserRoles, 2)
	if store.Exists(key) {
		indexed, err := client.SIsMember(t.Context(), KeyUserRolesIndex, key).Result()
		if err != nil || !indexed {
			t.Fatalf("concurrent cache write became unindexed: indexed=%v err=%v", indexed, err)
		}
	}
}

type cacheIndexBoundaryHook struct{ after func() error }

func (h *cacheIndexBoundaryHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h *cacheIndexBoundaryHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if err := next(ctx, cmd); err != nil {
			return err
		}
		return h.after()
	}
}
func (h *cacheIndexBoundaryHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		if err := next(ctx, cmds); err != nil {
			return err
		}
		return h.after()
	}
}

func TestSessionInvalidationIsScopedToUser(t *testing.T) {
	store := setupCacheTestRedis(t)
	svc := NewCacheService()
	for _, id := range []uint{1, 2} {
		if err := svc.SetConsoleSessionContext(t.Context(), fmt.Sprintf("session-%d", id), ConsoleSessionIdentity{UserID: id, TenantID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.DelConsoleSessionsForUsersContext(t.Context(), []uint{1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.GetConsoleSessionContext(t.Context(), "session-1"); ok {
		t.Fatal("revoked user's session remains cached")
	}
	if _, ok := svc.GetConsoleSessionContext(t.Context(), "session-2"); !ok {
		t.Fatal("other tenant's session was removed")
	}
	members, err := store.Members(KeyConsoleSessionIndex)
	if err != nil || len(members) != 1 {
		t.Fatalf("global index was not updated: %v %v", members, err)
	}
}

func TestConsoleIndexOutlivesPayloadAfterTTLReduction(t *testing.T) {
	store := setupCacheTestRedis(t)
	svc := NewCacheService()
	t.Setenv(EnvConsoleSessionCacheTTL, "300")
	if err := svc.SetConsoleSessionContext(t.Context(), "long-lived", ConsoleSessionIdentity{UserID: 1}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConsoleSessionCacheTTL, "1")
	if err := svc.SetConsoleSessionContext(t.Context(), "short-lived", ConsoleSessionIdentity{UserID: 2}); err != nil {
		t.Fatal(err)
	}
	store.FastForward(2 * time.Minute)
	if _, ok := svc.GetConsoleSessionContext(t.Context(), "long-lived"); !ok {
		t.Fatal("long-lived session expired early")
	}
	if err := svc.DelAllConsoleSessionsContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.GetConsoleSessionContext(t.Context(), "long-lived"); ok {
		t.Fatal("TTL reduction lost long-lived session index")
	}
}

func TestIndexInvalidationHonorsCancellation(t *testing.T) {
	store := setupCacheTestRedis(t)
	svc := NewCacheService()
	if err := svc.SetUserRolesContext(t.Context(), 1, []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := svc.DelAllUserRolesContext(ctx); err == nil {
		t.Fatal("canceled invalidation succeeded")
	}
	if !store.Exists(fmt.Sprintf(KeyUserRoles, 1)) {
		t.Fatal("canceled invalidation removed data")
	}
}
