package system

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/server/internal/config"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestOnlineUserStructDoesNotExposeAccessTokenField(t *testing.T) {
	if _, ok := reflect.TypeOf(OnlineUser{}).FieldByName("AccessToken"); ok {
		t.Fatal("OnlineUser should not expose a legacy plain access token field")
	}
}

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

func TestForceLogoutUsesUserIndexWithoutWalkingMainIndex(t *testing.T) {
	setupOnlineUserTestRedis(t)
	setOnlineUserJWTTestConfig(t)

	service := &OnlineUserService{}
	targetAccessToken, targetTokenID, targetExpiresAt := mustAccessToken(t, 7, "alice")
	sameUserAccessToken, sameUserTokenID, sameUserExpiresAt := mustAccessToken(t, 7, "alice")

	for _, user := range []OnlineUser{
		{UserID: 7, Username: "alice", TokenID: targetTokenID, AccessTokenExpiresAt: targetExpiresAt},
		{UserID: 7, Username: "alice", TokenID: sameUserTokenID, AccessTokenExpiresAt: sameUserExpiresAt},
	} {
		if err := service.SetOnlineUser(user, time.Hour); err != nil {
			t.Fatalf("set online user %s: %v", user.TokenID, err)
		}
	}

	ctx := context.Background()
	if err := redisstore.Client.ZRem(ctx, onlineUserIndexKey, sameUserTokenID).Err(); err != nil {
		t.Fatalf("remove same-user token from main index: %v", err)
	}
	if err := redisstore.Client.Set(ctx, onlineUserKey("corrupt-other-user-token"), "{not-json", time.Hour).Err(); err != nil {
		t.Fatalf("set unrelated corrupt payload: %v", err)
	}
	if err := redisstore.Client.ZAdd(ctx, onlineUserIndexKey, goredis.Z{
		Score:  float64(time.Now().Add(time.Hour).Unix()),
		Member: "corrupt-other-user-token",
	}).Err(); err != nil {
		t.Fatalf("index unrelated corrupt payload: %v", err)
	}

	if err := service.ForceLogout(targetTokenID); err != nil {
		t.Fatalf("force logout: %v", err)
	}

	if service.IsUserOnline(sameUserTokenID) {
		t.Fatal("same user's other session should be removed through the user index")
	}
	if !jwtpkg.IsTokenBlacklisted(targetAccessToken) {
		t.Fatal("target access token should be blacklisted")
	}
	if !jwtpkg.IsTokenBlacklisted(sameUserAccessToken) {
		t.Fatal("same user's access token should be blacklisted")
	}
	if _, err := redisstore.Client.ZScore(ctx, onlineUserIndexKey, "corrupt-other-user-token").Result(); err != nil {
		t.Fatalf("unrelated corrupt main-index entry should not be traversed or pruned by force logout: %v", err)
	}
}

func TestSetOnlineUserDoesNotStorePlainAccessToken(t *testing.T) {
	setupOnlineUserTestRedis(t)
	setOnlineUserJWTTestConfig(t)

	service := &OnlineUserService{}
	_, tokenID, expiresAt := mustAccessToken(t, 7, "alice")
	user := OnlineUser{
		UserID:               7,
		Username:             "alice",
		TokenID:              tokenID,
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
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("decode online user: %v", err)
	}
	if _, ok := stored["access_token"]; ok {
		t.Fatal("stored online user should not include access_token")
	}
}

func TestOnlineUserServiceWithClientUsesInjectedClient(t *testing.T) {
	globalStore := setupOnlineUserTestRedis(t)

	injectedStore, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start injected miniredis: %v", err)
	}
	injectedClient := goredis.NewClient(&goredis.Options{Addr: injectedStore.Addr()})
	t.Cleanup(func() {
		_ = injectedClient.Close()
		injectedStore.Close()
	})

	service := NewOnlineUserServiceWithClient(injectedClient)
	user := OnlineUser{UserID: 7, Username: "alice", TokenID: "injected-token"}
	if err := service.SetOnlineUserContext(context.Background(), user, time.Hour); err != nil {
		t.Fatalf("SetOnlineUserContext(): %v", err)
	}

	key := onlineUserKey(user.TokenID)
	if !injectedStore.Exists(key) {
		t.Fatalf("injected online user key %q was not written", key)
	}
	if globalStore.Exists(key) {
		t.Fatalf("global online user key %q was written; expected injected client only", key)
	}

	count, err := service.GetOnlineUserCountContext(context.Background())
	if err != nil {
		t.Fatalf("GetOnlineUserCountContext(): %v", err)
	}
	if count != 1 {
		t.Fatalf("online user count = %d, want 1", count)
	}
	if !service.IsUserOnlineContext(context.Background(), user.TokenID) {
		t.Fatal("injected user should be online")
	}

	if err := service.RemoveOnlineUserContext(context.Background(), user.TokenID); err != nil {
		t.Fatalf("RemoveOnlineUserContext(): %v", err)
	}
	if injectedStore.Exists(key) {
		t.Fatal("injected online user key still exists after remove")
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

func TestSetAndRemoveOnlineUserMaintainsUserIndex(t *testing.T) {
	setupOnlineUserTestRedis(t)

	service := &OnlineUserService{}
	user := OnlineUser{UserID: 7, Username: "alice", TokenID: "token-a"}
	if err := service.SetOnlineUser(user, time.Hour); err != nil {
		t.Fatalf("set online user: %v", err)
	}

	ctx := context.Background()
	if _, err := redisstore.Client.ZScore(ctx, testOnlineUserUserIndexKey(user.UserID), user.TokenID).Result(); err != nil {
		t.Fatalf("user online index missing token: %v", err)
	}

	if err := service.RemoveOnlineUser(user.TokenID); err != nil {
		t.Fatalf("remove online user: %v", err)
	}
	if _, err := redisstore.Client.ZScore(ctx, testOnlineUserUserIndexKey(user.UserID), user.TokenID).Result(); !errors.Is(err, goredis.Nil) {
		t.Fatalf("user online index token error = %v, want redis nil after remove", err)
	}
}

func TestOnlineUserCountUsesIndexCardinalityWithoutPayloadChecks(t *testing.T) {
	store := setupOnlineUserTestRedis(t)

	ctx := context.Background()
	const indexedUsers = 25
	score := float64(time.Now().Add(time.Hour).Unix())
	indexedTokens := make([]goredis.Z, 0, indexedUsers)
	for i := range indexedUsers {
		indexedTokens = append(indexedTokens, goredis.Z{
			Score:  score,
			Member: "indexed-token-" + strconv.Itoa(i),
		})
	}
	if err := redisstore.Client.ZAdd(ctx, onlineUserIndexKey, indexedTokens...).Err(); err != nil {
		t.Fatalf("index online users: %v", err)
	}

	commandsBefore := store.CommandCount()
	count, err := (&OnlineUserService{}).GetOnlineUserCount()
	if err != nil {
		t.Fatalf("get online user count: %v", err)
	}
	if count != indexedUsers {
		t.Fatalf("online user count = %d, want indexed count %d", count, indexedUsers)
	}
	if commands := store.CommandCount() - commandsBefore; commands > 3 {
		t.Fatalf("online user count used %d redis commands, want constant-time index count", commands)
	}
}

func TestOnlineUserIndexPrunesExpiredSessions(t *testing.T) {
	setupOnlineUserTestRedis(t)

	service := &OnlineUserService{}
	if err := service.SetOnlineUser(OnlineUser{
		UserID:               7,
		Username:             "alice",
		TokenID:              "token-a",
		AccessTokenExpiresAt: time.Now().Add(-time.Second),
	}, time.Hour); err != nil {
		t.Fatalf("set online user: %v", err)
	}

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

func TestForceLogoutPrunesExpiredUserIndexEntries(t *testing.T) {
	setupOnlineUserTestRedis(t)
	setOnlineUserJWTTestConfig(t)

	service := &OnlineUserService{}
	activeAccessToken, activeTokenID, activeExpiresAt := mustAccessToken(t, 7, "alice")
	expiredTokenID := "expired-token"

	if err := service.SetOnlineUser(OnlineUser{
		UserID:               7,
		Username:             "alice",
		TokenID:              activeTokenID,
		AccessTokenExpiresAt: activeExpiresAt,
	}, time.Hour); err != nil {
		t.Fatalf("set active online user: %v", err)
	}
	if err := redisstore.Client.ZAdd(context.Background(), testOnlineUserUserIndexKey(7), goredis.Z{
		Score:  float64(time.Now().Add(-time.Minute).Unix()),
		Member: expiredTokenID,
	}).Err(); err != nil {
		t.Fatalf("seed expired user index entry: %v", err)
	}

	if err := service.ForceLogout(activeTokenID); err != nil {
		t.Fatalf("force logout: %v", err)
	}

	if !jwtpkg.IsTokenBlacklisted(activeAccessToken) {
		t.Fatal("active access token should be blacklisted")
	}
	if _, err := redisstore.Client.ZScore(context.Background(), testOnlineUserUserIndexKey(7), expiredTokenID).Result(); !errors.Is(err, goredis.Nil) {
		t.Fatalf("expired user index token error = %v, want redis nil after force logout", err)
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

func testOnlineUserUserIndexKey(userID uint) string {
	return "online_users:user:" + strconv.FormatUint(uint64(userID), 10)
}
