package jwt

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/services/audit/internal/config"
	redisstore "github.com/go-admin-kit/services/audit/internal/pkg/redis"
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

	if IsTokenIDBlacklistedContext(context.Background(), "token-id") {
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

	claims, err := ParseTokenContext(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("parse token before revoke: %v", err)
	}
	if IsTokenBlacklistedContext(context.Background(), accessToken) {
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
	if !IsTokenBlacklistedContext(context.Background(), accessToken) {
		t.Fatal("token should be reported as blacklisted")
	}

	_, err = ParseTokenContext(context.Background(), accessToken)
	if !errors.Is(err, ErrRevokedToken) {
		t.Fatalf("parse revoked token error = %v, want %v", err, ErrRevokedToken)
	}
}

func TestRevokeTokenUsesInjectedBlacklistStore(t *testing.T) {
	setJWTTestConfig(t)

	oldRedis := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldRedis
	})

	store := &stubTokenBlacklistStore{
		values: make(map[string]time.Duration),
	}
	restore := SetTokenBlacklistStore(store)
	t.Cleanup(restore)

	accessToken, _, err := GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := ParseTokenContext(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("parse token before revoke: %v", err)
	}

	if err := RevokeToken(accessToken, claims); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if store.setTokenID != claims.ID {
		t.Fatalf("blacklist token id = %q, want %q", store.setTokenID, claims.ID)
	}
	if store.values[claims.ID] <= 0 {
		t.Fatalf("blacklist ttl = %s, want positive", store.values[claims.ID])
	}

	_, err = ParseTokenContext(context.Background(), accessToken)
	if !errors.Is(err, ErrRevokedToken) {
		t.Fatalf("parse revoked token error = %v, want %v", err, ErrRevokedToken)
	}
}

func TestContextAPIsPassContextToBlacklistStore(t *testing.T) {
	setupJWTTestRedis(t)
	setJWTTestConfig(t)

	const contextValue = "request-ctx"
	ctx := context.WithValue(context.Background(), stubContextKey{}, contextValue)
	store := &stubTokenBlacklistStore{
		values: make(map[string]time.Duration),
	}
	restore := SetTokenBlacklistStore(store)
	t.Cleanup(restore)

	accessToken, refreshToken, err := GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	accessClaims, err := ParseTokenContext(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}

	if err := BlacklistTokenIDContext(ctx, "token-id", time.Minute); err != nil {
		t.Fatalf("BlacklistTokenIDContext() error = %v", err)
	}
	if store.setContextValue != contextValue {
		t.Fatalf("BlacklistTokenIDContext() context value = %v, want %q", store.setContextValue, contextValue)
	}

	store.setContextValue = nil
	if err := BlacklistTokenContext(ctx, accessToken, time.Minute); err != nil {
		t.Fatalf("BlacklistTokenContext() error = %v", err)
	}
	if store.setContextValue != contextValue {
		t.Fatalf("BlacklistTokenContext() context value = %v, want %q", store.setContextValue, contextValue)
	}

	store.setContextValue = nil
	if err := RevokeTokenContext(ctx, accessToken, accessClaims); err != nil {
		t.Fatalf("RevokeTokenContext() error = %v", err)
	}
	if store.setContextValue != contextValue {
		t.Fatalf("RevokeTokenContext() context value = %v, want %q", store.setContextValue, contextValue)
	}

	store.setContextValue = nil
	store.hasContextValue = nil
	if _, _, err := RefreshTokenContext(ctx, refreshToken); err != nil {
		t.Fatalf("RefreshTokenContext() error = %v", err)
	}
	if store.hasContextValue != contextValue {
		t.Fatalf("RefreshTokenContext() revocation check context value = %v, want %q", store.hasContextValue, contextValue)
	}
	if store.setContextValue != contextValue {
		t.Fatalf("RefreshTokenContext() revoke context value = %v, want %q", store.setContextValue, contextValue)
	}
}

func TestConsumeTokenIDUsesInjectedBlacklistStore(t *testing.T) {
	store := &stubTokenBlacklistStore{
		values: make(map[string]time.Duration),
	}
	restore := SetTokenBlacklistStore(store)
	t.Cleanup(restore)

	consumed, err := ConsumeTokenID(context.Background(), "challenge-id", time.Minute)
	if err != nil {
		t.Fatalf("ConsumeTokenID() error = %v", err)
	}
	if !consumed {
		t.Fatal("first ConsumeTokenID() should consume the token id")
	}

	consumed, err = ConsumeTokenID(context.Background(), "challenge-id", time.Minute)
	if err != nil {
		t.Fatalf("second ConsumeTokenID() error = %v", err)
	}
	if consumed {
		t.Fatal("second ConsumeTokenID() should report an already consumed token id")
	}
}

func TestGenerateTokenWithAccessTTLUsesCustomTTL(t *testing.T) {
	setupJWTTestRedis(t)
	setJWTTestConfig(t)

	accessToken, refreshToken, err := GenerateTokenWithAccessTTL(42, "alice", 8*time.Hour)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	accessClaims, err := ParseTokenContext(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	refreshClaims, err := ParseTokenContext(context.Background(), refreshToken)
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

func TestGenerateWebSocketTicketUsesDedicatedTokenType(t *testing.T) {
	setupJWTTestRedis(t)
	setJWTTestConfig(t)

	ticket, err := GenerateWebSocketTicket(42, "alice", time.Minute)
	if err != nil {
		t.Fatalf("GenerateWebSocketTicket() error = %v", err)
	}

	claims, err := ParseWebSocketTicket(ticket)
	if err != nil {
		t.Fatalf("ParseWebSocketTicket() error = %v", err)
	}
	if claims.TokenType != WebSocketTicketTokenType {
		t.Fatalf("token type = %q, want %q", claims.TokenType, WebSocketTicketTokenType)
	}
	if claims.UserID != 42 || claims.Username != "alice" {
		t.Fatalf("claims user = %d/%s, want 42/alice", claims.UserID, claims.Username)
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

type stubTokenBlacklistStore struct {
	values          map[string]time.Duration
	setTokenID      string
	setContextValue any
	hasContextValue any
}

func (s *stubTokenBlacklistStore) SetTokenID(ctx context.Context, tokenID string, expireTime time.Duration) error {
	s.setTokenID = tokenID
	s.setContextValue = ctx.Value(stubContextKey{})
	s.values[tokenID] = expireTime
	return nil
}

func (s *stubTokenBlacklistStore) HasTokenID(ctx context.Context, tokenID string) (bool, error) {
	s.hasContextValue = ctx.Value(stubContextKey{})
	_, ok := s.values[tokenID]
	return ok, nil
}

type stubContextKey struct{}
