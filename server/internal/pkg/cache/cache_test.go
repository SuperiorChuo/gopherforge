package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/server/internal/config"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestJWTBlacklistUsesTokenIDKey(t *testing.T) {
	store := setupCacheTestRedis(t)
	setCacheJWTTestConfig(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := jwtpkg.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	service := NewCacheService()
	if err := service.AddJWTToBlacklist(accessToken, time.Hour); err != nil {
		t.Fatalf("add jwt to blacklist: %v", err)
	}

	key := fmt.Sprintf(KeyJWTBlacklist, claims.ID)
	if !store.Exists(key) {
		t.Fatalf("blacklist key %q was not written", key)
	}
	if store.Exists(fmt.Sprintf(KeyJWTBlacklist, accessToken)) {
		t.Fatal("cache blacklist should not use the full token as the redis key")
	}
	if !service.IsJWTInBlacklist(accessToken) {
		t.Fatal("token should be reported as blacklisted")
	}
	if err := service.RemoveJWTFromBlacklist(accessToken); err != nil {
		t.Fatalf("remove jwt from blacklist: %v", err)
	}
	if service.IsJWTInBlacklist(accessToken) {
		t.Fatal("token should not remain blacklisted")
	}
}

func TestNewCacheServiceWithClientUsesInjectedClient(t *testing.T) {
	globalStore := setupCacheTestRedis(t)

	injectedStore, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start injected miniredis: %v", err)
	}
	injectedClient := goredis.NewClient(&goredis.Options{Addr: injectedStore.Addr()})
	t.Cleanup(func() {
		_ = injectedClient.Close()
		injectedStore.Close()
	})

	service := NewCacheServiceWithClient(injectedClient)
	if err := service.SetLoginCaptchaContext(context.Background(), "captcha-key", "2468"); err != nil {
		t.Fatalf("SetLoginCaptchaContext(): %v", err)
	}

	key := fmt.Sprintf(KeyLoginCaptcha, "captcha-key")
	if !injectedStore.Exists(key) {
		t.Fatalf("injected cache key %q was not written", key)
	}
	if globalStore.Exists(key) {
		t.Fatalf("global cache key %q was written; expected injected client only", key)
	}
}

func TestDelUserPermissionsBatchContextHonorsCanceledContext(t *testing.T) {
	setupCacheTestRedis(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewCacheService().DelUserPermissionsBatchContext(ctx, []uint{1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DelUserPermissionsBatchContext() error = %v, want context.Canceled", err)
	}
}

func TestDelAllUserPermissionsContextDeletesIndexedKeysWithoutPatternScan(t *testing.T) {
	setupCacheTestRedis(t)

	indexKey := KeyUserPermissionsIndex
	indexedKey := fmt.Sprintf(KeyUserPermissions, 101)
	unindexedKey := fmt.Sprintf(KeyUserPermissions, 202)

	if err := redisstore.Client.SAdd(context.Background(), indexedKey, "system:user:list").Err(); err != nil {
		t.Fatalf("seed indexed permission key: %v", err)
	}
	if err := redisstore.Client.SAdd(context.Background(), unindexedKey, "system:role:list").Err(); err != nil {
		t.Fatalf("seed unindexed permission key: %v", err)
	}
	if err := redisstore.Client.SAdd(context.Background(), indexKey, indexedKey).Err(); err != nil {
		t.Fatalf("seed permissions index: %v", err)
	}

	if err := NewCacheService().DelAllUserPermissionsContext(context.Background()); err != nil {
		t.Fatalf("DelAllUserPermissionsContext(): %v", err)
	}

	if redisstore.Client.Exists(context.Background(), indexedKey).Val() != 0 {
		t.Fatalf("indexed permission key %q was not deleted", indexedKey)
	}
	if redisstore.Client.Exists(context.Background(), indexKey).Val() != 0 {
		t.Fatalf("permission index key %q was not deleted", indexKey)
	}
	if redisstore.Client.Exists(context.Background(), unindexedKey).Val() == 0 {
		t.Fatalf("unindexed permission key %q was deleted; DelAllUserPermissionsContext should use the index, not a pattern scan", unindexedKey)
	}
}

func TestSetUserPermissionsContextMaintainsPermissionIndex(t *testing.T) {
	setupCacheTestRedis(t)

	service := NewCacheService()
	key := fmt.Sprintf(KeyUserPermissions, 42)
	indexKey := KeyUserPermissionsIndex

	if err := service.SetUserPermissionsContext(context.Background(), 42, []string{"system:user:list"}); err != nil {
		t.Fatalf("SetUserPermissionsContext(non-empty): %v", err)
	}
	if ok := redisstore.Client.SIsMember(context.Background(), key, "system:user:list").Val(); !ok {
		t.Fatalf("permission key %q does not contain expected permission after non-empty set", key)
	}
	if ok := redisstore.Client.SIsMember(context.Background(), indexKey, key).Val(); !ok {
		t.Fatalf("permission index %q does not contain %q after non-empty set", indexKey, key)
	}

	if err := service.SetUserPermissionsContext(context.Background(), 42, nil); err != nil {
		t.Fatalf("SetUserPermissionsContext(empty): %v", err)
	}
	if redisstore.Client.Exists(context.Background(), key).Val() != 0 {
		t.Fatalf("permission key %q still exists after empty set", key)
	}
	if ok := redisstore.Client.SIsMember(context.Background(), indexKey, key).Val(); ok {
		t.Fatalf("permission index %q still contains %q after empty set", indexKey, key)
	}
}

func TestDelUserPermissionsContextRemovesPermissionIndexMember(t *testing.T) {
	setupCacheTestRedis(t)

	service := NewCacheService()
	key := fmt.Sprintf(KeyUserPermissions, 42)
	indexKey := KeyUserPermissionsIndex

	if err := service.SetUserPermissionsContext(context.Background(), 42, []string{"system:user:list"}); err != nil {
		t.Fatalf("SetUserPermissionsContext(): %v", err)
	}
	if err := service.DelUserPermissionsContext(context.Background(), 42); err != nil {
		t.Fatalf("DelUserPermissionsContext(): %v", err)
	}

	if ok := redisstore.Client.SIsMember(context.Background(), indexKey, key).Val(); ok {
		t.Fatalf("permission index %q still contains %q after delete", indexKey, key)
	}
}

func TestDelUserPermissionsBatchContextRemovesPermissionIndexMembers(t *testing.T) {
	setupCacheTestRedis(t)

	service := NewCacheService()
	indexKey := KeyUserPermissionsIndex
	deletedKey := fmt.Sprintf(KeyUserPermissions, 42)
	keptKey := fmt.Sprintf(KeyUserPermissions, 99)

	if err := service.SetUserPermissionsContext(context.Background(), 42, []string{"system:user:list"}); err != nil {
		t.Fatalf("SetUserPermissionsContext(42): %v", err)
	}
	if err := service.SetUserPermissionsContext(context.Background(), 99, []string{"system:role:list"}); err != nil {
		t.Fatalf("SetUserPermissionsContext(99): %v", err)
	}
	if err := service.DelUserPermissionsBatchContext(context.Background(), []uint{42}); err != nil {
		t.Fatalf("DelUserPermissionsBatchContext(): %v", err)
	}

	if ok := redisstore.Client.SIsMember(context.Background(), indexKey, deletedKey).Val(); ok {
		t.Fatalf("permission index %q still contains deleted key %q", indexKey, deletedKey)
	}
	if ok := redisstore.Client.SIsMember(context.Background(), indexKey, keptKey).Val(); !ok {
		t.Fatalf("permission index %q lost untouched key %q", indexKey, keptKey)
	}
}

func TestCacheContextMethodsHonorCanceledContext(t *testing.T) {
	setupCacheTestRedis(t)
	setCacheJWTTestConfig(t)

	accessToken, _, err := jwtpkg.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := NewCacheService()
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "add jwt blacklist",
			run: func() error {
				return service.AddJWTToBlacklistContext(ctx, accessToken, time.Hour)
			},
		},
		{
			name: "remove jwt blacklist",
			run: func() error {
				return service.RemoveJWTFromBlacklistContext(ctx, accessToken)
			},
		},
		{
			name: "set login captcha",
			run: func() error {
				return service.SetLoginCaptchaContext(ctx, "captcha-key", "1234")
			},
		},
		{
			name: "set user permissions",
			run: func() error {
				return service.SetUserPermissionsContext(ctx, 42, []string{"system:user:list"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func setupCacheTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})

	return store
}

func setCacheJWTTestConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.Cfg.JWT
	config.Cfg.JWT = config.JWTConfig{
		Secret:               "unit-test-secret-at-least-32-characters",
		AccessTokenExpire:    3600,
		RefreshTokenExpire:   7200,
		RefreshTokenRotation: true,
		Issuer:               "unit-test",
	}

	t.Cleanup(func() {
		config.Cfg.JWT = oldConfig
	})
}
