package system

import (
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
	userAccessTokenA, userAccessTokenExpiresAtA := mustAccessToken(t, 7, "alice")
	userAccessTokenB, userAccessTokenExpiresAtB := mustAccessToken(t, 7, "alice")
	otherAccessToken, otherAccessTokenExpiresAt := mustAccessToken(t, 8, "bob")

	onlineUsers := []OnlineUser{
		{
			UserID:               7,
			Username:             "alice",
			TokenID:              "session-a",
			AccessToken:          userAccessTokenA,
			AccessTokenExpiresAt: userAccessTokenExpiresAtA,
		},
		{
			UserID:               7,
			Username:             "alice",
			TokenID:              "session-b",
			AccessToken:          userAccessTokenB,
			AccessTokenExpiresAt: userAccessTokenExpiresAtB,
		},
		{
			UserID:               8,
			Username:             "bob",
			TokenID:              "session-other",
			AccessToken:          otherAccessToken,
			AccessTokenExpiresAt: otherAccessTokenExpiresAt,
		},
	}

	for _, user := range onlineUsers {
		if err := service.SetOnlineUser(user, time.Hour); err != nil {
			t.Fatalf("set online user %s: %v", user.TokenID, err)
		}
	}

	if err := service.ForceLogout("session-a"); err != nil {
		t.Fatalf("force logout: %v", err)
	}

	if service.IsUserOnline("session-a") {
		t.Fatal("target session should be removed")
	}
	if service.IsUserOnline("session-b") {
		t.Fatal("same user's other session should be removed")
	}
	if !service.IsUserOnline("session-other") {
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

func mustAccessToken(t *testing.T, userID uint, username string) (string, time.Time) {
	t.Helper()

	accessToken, _, err := jwtpkg.GenerateToken(userID, username)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := jwtpkg.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	return accessToken, claims.ExpiresAt.Time
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
