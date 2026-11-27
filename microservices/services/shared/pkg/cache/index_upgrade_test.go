package cache

import (
	"fmt"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func setIndexExpiryMode(t *testing.T, addr, mode string) {
	t.Helper()
	script, err := os.ReadFile("index_expiry_migration.lua")
	if err != nil {
		t.Fatal(err)
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	defer client.Close()
	if _, err := client.Eval(t.Context(), string(script), nil, mode).Result(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyWriterDuringRollingUpgradeKeepsLiveIndex(t *testing.T) {
	store := setupCacheTestRedis(t)
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	svc := NewCacheServiceWithClient(client)
	if err := svc.SetUserRolesContext(ctx, 1, []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	store.FastForward(time.Hour)
	// The old replica writes a one-hour payload but only SADDs its index.
	legacyKey := fmt.Sprintf(KeyUserRoles, 2)
	pipe := client.TxPipeline()
	pipe.SAdd(ctx, legacyKey, "admin")
	pipe.Expire(ctx, legacyKey, time.Hour)
	pipe.SAdd(ctx, KeyUserRolesIndex, legacyKey)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	store.FastForward(2 * time.Minute)
	if !store.Exists(legacyKey) || !store.Exists(KeyUserRolesIndex) {
		t.Fatal("index expired before legacy writer's live payload")
	}
	if err := svc.DelAllUserRolesContext(ctx); err != nil {
		t.Fatal(err)
	}
	if store.Exists(legacyKey) {
		t.Fatal("legacy writer escaped permission invalidation")
	}
}

func TestExpiryActivationAndRollbackPrepare(t *testing.T) {
	store := setupCacheTestRedis(t)
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	svc := NewCacheServiceWithClient(client)
	if err := svc.SetUserPermissionsContext(ctx, 1, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	if ttl := store.TTL(KeyUserPermissionsIndex); ttl != 0 {
		t.Fatalf("unactivated index has TTL=%v", ttl)
	}
	setIndexExpiryMode(t, store.Addr(), "activate")
	if ttl := store.TTL(KeyUserPermissionsIndex); ttl <= time.Hour {
		t.Fatalf("activation did not expire existing index safely: %v", ttl)
	}
	setIndexExpiryMode(t, store.Addr(), "prepare")
	if ttl := store.TTL(KeyUserPermissionsIndex); ttl != 0 {
		t.Fatalf("rollback prepare retained TTL=%v", ttl)
	}
	if store.Exists(cacheIndexExpiryReadyKey) {
		t.Fatal("rollback left activation enabled")
	}
	if err := svc.SetUserPermissionsContext(ctx, 2, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	if ttl := store.TTL(KeyUserPermissionsIndex); ttl != 0 {
		t.Fatalf("new writer reenabled TTL during rollback: %v", ttl)
	}
	if !store.Exists(fmt.Sprintf(KeyUserPermissions, 1)) {
		t.Fatal("migration deleted cached payload")
	}
	setIndexExpiryMode(t, store.Addr(), "activate")
	store.FastForward(2 * time.Hour)
	if store.Exists(KeyUserPermissionsIndex) {
		t.Fatal("activated idle legacy index did not expire")
	}
}
