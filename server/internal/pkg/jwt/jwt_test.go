package jwt

import (
	"errors"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/server/internal/config"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	jwtlib "github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"
)

func TestRevokeTokenRejectsInvalidClaimsAndIgnoresExpiredTokens(t *testing.T) {
	if err := RevokeToken("token", nil); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("nil claims error = %v, want %v", err, ErrInvalidToken)
	}

	if err := RevokeToken("token", &Claims{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("missing expiry error = %v, want %v", err, ErrInvalidToken)
	}

	expiredClaims := &Claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	if err := RevokeToken("expired-token", expiredClaims); err != nil {
		t.Fatalf("expired token revoke should be a no-op, got %v", err)
	}
}

func TestIsTokenIDBlacklistedWithoutRedisClient(t *testing.T) {
	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	if IsTokenIDBlacklisted("token-id") {
		t.Fatal("token should not be blacklisted when redis client is unavailable")
	}
}

func TestRevokeTokenBlacklistsUnexpiredToken(t *testing.T) {
	store := setupJWTTestRedis(t)
	setJWTTestConfig(t)

	accessToken, _, err := GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse token before revoke: %v", err)
	}
	if IsTokenBlacklisted(accessToken) {
		t.Fatal("token should not start blacklisted")
	}

	if err := RevokeToken(accessToken, claims); err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	key := fmt.Sprintf("jwt:blacklist:%s", claims.ID)
	if !store.Exists(key) {
		t.Fatalf("blacklist key %q was not written", key)
	}
	if store.Exists(fmt.Sprintf("jwt:blacklist:%s", accessToken)) {
		t.Fatal("blacklist should not use the full token as the redis key")
	}
	if !IsTokenBlacklisted(accessToken) {
		t.Fatal("token should be reported as blacklisted")
	}

	_, err = ParseToken(accessToken)
	if !errors.Is(err, ErrRevokedToken) {
		t.Fatalf("parse revoked token error = %v, want %v", err, ErrRevokedToken)
	}
}

func TestGenerateTokenWithAccessTTLUsesCustomTTL(t *testing.T) {
	setupJWTTestRedis(t)
	setJWTTestConfig(t)

	accessToken, refreshToken, err := GenerateTokenWithAccessTTL(42, "alice", 8*time.Hour)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	accessClaims, err := ParseToken(accessToken)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	refreshClaims, err := ParseToken(refreshToken)
	if err != nil {
		t.Fatalf("parse refresh token: %v", err)
	}
	if accessClaims.ExpiresAt == nil || accessClaims.IssuedAt == nil {
		t.Fatal("access token should include issued and expiry timestamps")
	}

	accessTTL := accessClaims.ExpiresAt.Sub(accessClaims.IssuedAt.Time)
	if accessTTL < 8*time.Hour-time.Second || accessTTL > 8*time.Hour+time.Second {
		t.Fatalf("access token ttl = %s, want about 8h", accessTTL)
	}
	refreshTTL := refreshClaims.ExpiresAt.Sub(refreshClaims.IssuedAt.Time)
	if refreshTTL < 2*time.Hour-time.Second || refreshTTL > 2*time.Hour+time.Second {
		t.Fatalf("refresh token ttl = %s, want about default 2h", refreshTTL)
	}
}

func setupJWTTestRedis(t *testing.T) *miniredis.Miniredis {
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

func setJWTTestConfig(t *testing.T) {
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
