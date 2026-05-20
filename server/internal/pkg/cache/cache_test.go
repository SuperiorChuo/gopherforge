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

func TestDelUserPermissionsBatchContextHonorsCanceledContext(t *testing.T) {
	setupCacheTestRedis(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewCacheService().DelUserPermissionsBatchContext(ctx, []uint{1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DelUserPermissionsBatchContext() error = %v, want context.Canceled", err)
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
