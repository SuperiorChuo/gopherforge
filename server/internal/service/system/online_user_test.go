package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/server/internal/config"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestForceLogoutRevokesAndRemovesAllSessionsForUser(t *testing.T) {
	setupOnlineUserTestRedis(t)
	setOnlineUserJWTTestConfig(t)

	service := &OnlineUserService{}
	userAccessTokenA, userAccessTokenIDA, userAccessTokenExpiresAtA := mustAccessToken(t, 7, "alice")
	userAccessTokenB, userAccessTokenIDB, userAccessTokenExpiresAtB := mustAccessToken(t, 7, "alice")
	otherAccessToken, otherAccessTokenID, otherAccessTokenExpiresAt := mustAccessToken(t, 8, "bob")

	onlineUsers := []OnlineUser{
		{
			UserID:               7,
			Username:             "alice",
			TokenID:              userAccessTokenIDA,
			AccessTokenExpiresAt: userAccessTokenExpiresAtA,
		},
		{
			UserID:               7,
			Username:             "alice",
			TokenID:              userAccessTokenIDB,
			AccessTokenExpiresAt: userAccessTokenExpiresAtB,
		},
		{
			UserID:               8,
			Username:             "bob",
			TokenID:              otherAccessTokenID,
			AccessTokenExpiresAt: otherAccessTokenExpiresAt,
		},
	}

	for _, user := range onlineUsers {
		if err := service.SetOnlineUser(user, time.Hour); err != nil {
			t.Fatalf("set online user %s: %v", user.TokenID, err)
		}
	}

	if err := service.ForceLogout(userAccessTokenIDA); err != nil {
		t.Fatalf("force logout: %v", err)
	}

	if service.IsUserOnline(userAccessTokenIDA) {
		t.Fatal("target session should be removed")
	}
	if service.IsUserOnline(userAccessTokenIDB) {
		t.Fatal("same user's other session should be removed")
	}
	if !service.IsUserOnline(otherAccessTokenID) {
		t.Fatal("other user's session should remain online")
	}
	if !jwtpkg.IsTokenBlacklisted(userAccessTokenA) {
		t.Fatal("target access token should be blacklisted")
	}
	if !jwtpkg.IsTokenBlacklisted(userAccessTokenB) {
		t.Fatal("same user's other access token should be blacklisted")
	}
	if jwtpkg.IsTokenBlacklisted(otherAccessToken) {
		t.Fatal("other user's access token should not be blacklisted")
	}
}

func TestSetOnlineUserDoesNotStorePlainAccessToken(t *testing.T) {
	setupOnlineUserTestRedis(t)
	setOnlineUserJWTTestConfig(t)

	service := &OnlineUserService{}
	accessToken, tokenID, expiresAt := mustAccessToken(t, 7, "alice")
	user := OnlineUser{
		UserID:               7,
		Username:             "alice",
		TokenID:              tokenID,
		AccessToken:          accessToken,
		AccessTokenExpiresAt: expiresAt,
	}

	if err := service.SetOnlineUser(user, time.Hour); err != nil {
		t.Fatalf("set online user: %v", err)
	}

	raw, err := redisstore.Client.Get(context.Background(), onlineUserPrefix+tokenID).Result()
	if err != nil {
		t.Fatalf("get online user: %v", err)
	}
	if json.Valid([]byte(raw)) == false {
		t.Fatalf("stored online user should be valid json: %q", raw)
	}
	if strings.Contains(raw, accessToken) {
		t.Fatal("stored online user should not contain the access token")
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("decode online user: %v", err)
	}
	if _, ok := stored["access_token"]; ok {
		t.Fatal("stored online user should not include access_token")
	}
}

func TestOnlineUsersAreIndexedForListAndCount(t *testing.T) {
	setupOnlineUserTestRedis(t)

	service := &OnlineUserService{}
	users := []OnlineUser{
		{UserID: 7, Username: "alice", TokenID: "token-a"},
		{UserID: 8, Username: "bob", TokenID: "token-b"},
	}
	for _, user := range users {
		if err := service.SetOnlineUser(user, time.Hour); err != nil {
			t.Fatalf("set online user %s: %v", user.TokenID, err)
		}
	}

	ctx := context.Background()
	if _, err := redisstore.Client.ZScore(ctx, onlineUserIndexKey, "token-a").Result(); err != nil {
		t.Fatalf("online user index missing token-a: %v", err)
	}

	list, err := service.GetOnlineUsers()
	if err != nil {
		t.Fatalf("get online users: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("online users len = %d, want 2", len(list))
	}

	count, err := service.GetOnlineUserCount()
	if err != nil {
		t.Fatalf("get online user count: %v", err)
	}
	if count != 2 {
		t.Fatalf("online user count = %d, want 2", count)
	}
}

func TestOnlineUserCountUsesIndexWithoutDecodingPayloads(t *testing.T) {
	setupOnlineUserTestRedis(t)

	ctx := context.Background()
	if err := redisstore.Client.Set(ctx, onlineUserKey("token-a"), "{not-json", time.Hour).Err(); err != nil {
		t.Fatalf("set corrupt online user payload: %v", err)
	}
	if err := redisstore.Client.ZAdd(ctx, onlineUserIndexKey, goredis.Z{
		Score:  float64(time.Now().Add(time.Hour).Unix()),
		Member: "token-a",
	}).Err(); err != nil {
		t.Fatalf("index online user: %v", err)
	}

	count, err := (&OnlineUserService{}).GetOnlineUserCount()
	if err != nil {
		t.Fatalf("get online user count: %v", err)
	}
	if count != 1 {
		t.Fatalf("online user count = %d, want indexed count 1", count)
	}
}

func TestOnlineUserIndexPrunesExpiredSessions(t *testing.T) {
	store := setupOnlineUserTestRedis(t)

	service := &OnlineUserService{}
	if err := service.SetOnlineUser(OnlineUser{UserID: 7, Username: "alice", TokenID: "token-a"}, time.Second); err != nil {
		t.Fatalf("set online user: %v", err)
	}

	store.FastForward(2 * time.Second)

	count, err := service.GetOnlineUserCount()
	if err != nil {
		t.Fatalf("get online user count: %v", err)
	}
	if count != 0 {
		t.Fatalf("online user count = %d, want 0 after expiration", count)
	}

	zcard, err := redisstore.Client.ZCard(context.Background(), onlineUserIndexKey).Result()
	if err != nil {
		t.Fatalf("zcard online user index: %v", err)
	}
	if zcard != 0 {
		t.Fatalf("online user index size = %d, want 0 after pruning", zcard)
	}
}

func TestOnlineUserContextMethodsHonorCanceledContext(t *testing.T) {
	setupOnlineUserTestRedis(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := &OnlineUserService{}
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "set online user",
			run: func() error {
				return service.SetOnlineUserContext(ctx, OnlineUser{UserID: 7, Username: "alice", TokenID: "token-a"}, time.Hour)
			},
		},
		{
			name: "remove online user",
			run: func() error {
				return service.RemoveOnlineUserContext(ctx, "token-a")
			},
		},
		{
			name: "get online users",
			run: func() error {
				_, err := service.GetOnlineUsersContext(ctx)
				return err
			},
		},
		{
			name: "force logout",
			run: func() error {
				return service.ForceLogoutContext(ctx, "token-a")
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

func mustAccessToken(t *testing.T, userID uint, username string) (string, string, time.Time) {
	t.Helper()

	accessToken, _, err := jwtpkg.GenerateToken(userID, username)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := jwtpkg.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	return accessToken, claims.ID, claims.ExpiresAt.Time
}

func setupOnlineUserTestRedis(t *testing.T) *miniredis.Miniredis {
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

func setOnlineUserJWTTestConfig(t *testing.T) {
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
