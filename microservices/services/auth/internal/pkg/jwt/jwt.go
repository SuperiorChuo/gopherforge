package jwt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/redis"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrExpiredToken   = errors.New("token expired")
	ErrTokenNotFound  = errors.New("token not found")
	ErrRevokedToken   = errors.New("token revoked")
	ErrWrongTokenType = errors.New("wrong token type")
)

const (
	AccessTokenType          = "access"
	RefreshTokenType         = "refresh"
	TOTPChallengeTokenType   = "totp_challenge"
	WebSocketTicketTokenType = "ws_ticket"
)

// TokenBlacklistStore stores revoked JWT token IDs.
type TokenBlacklistStore interface {
	SetTokenID(ctx context.Context, tokenID string, expireTime time.Duration) error
	HasTokenID(ctx context.Context, tokenID string) (bool, error)
}

type tokenIDConsumer interface {
	ConsumeTokenID(ctx context.Context, tokenID string, expireTime time.Duration) (bool, error)
}

var (
	tokenBlacklistStoreMu sync.RWMutex
	tokenBlacklistStore   TokenBlacklistStore = redisTokenBlacklistStore{}
)

type redisTokenBlacklistStore struct{}

type Claims struct {
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	TenantID       uint   `json:"tenant_id"`
	PlatformAdmin  bool   `json:"platform_admin"`
	TokenType      string `json:"token_type"`
	jwtlib.RegisteredClaims
}

// NormalizeTenantID maps zero/empty tenant to default tenant (id=1).
func NormalizeTenantID(id uint) uint {
	if id == 0 {
		return 1
	}
	return id
}

// SetTokenBlacklistStore replaces the package-level blacklist store and returns a restore function.
func SetTokenBlacklistStore(store TokenBlacklistStore) func() {
	tokenBlacklistStoreMu.Lock()
	previous := tokenBlacklistStore
	if store == nil {
		tokenBlacklistStore = redisTokenBlacklistStore{}
	} else {
		tokenBlacklistStore = store
	}
	tokenBlacklistStoreMu.Unlock()

	return func() {
		tokenBlacklistStoreMu.Lock()
		tokenBlacklistStore = previous
		tokenBlacklistStoreMu.Unlock()
	}
}

func GenerateToken(userID uint, username string) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	return GenerateTokenWithTenantAndAccessTTL(userID, username, 1, time.Duration(cfg.AccessTokenExpire)*time.Second)
}

func GenerateTokenWithAccessTTL(userID uint, username string, accessTTL time.Duration) (accessToken, refreshToken string, err error) {
	return GenerateTokenWithTenantAndAccessTTL(userID, username, 1, accessTTL)
}

// GenerateTokenWithTenant mints access+refresh for a tenant-scoped user.
func GenerateTokenWithTenant(userID uint, username string, tenantID uint) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	return GenerateTokenWithTenantAndAccessTTL(userID, username, tenantID, time.Duration(cfg.AccessTokenExpire)*time.Second)
}

func GenerateTokenWithTenantAndAccessTTL(userID uint, username string, tenantID uint, accessTTL time.Duration) (accessToken, refreshToken string, err error) {
	return GenerateTokenWithTenantPlatformAndAccessTTL(userID, username, tenantID, false, accessTTL)
}

// GenerateTokenWithTenantPlatformAndAccessTTL mints tokens including platform operator flag (M4).
func GenerateTokenWithTenantPlatformAndAccessTTL(userID uint, username string, tenantID uint, platformAdmin bool, accessTTL time.Duration) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	now := time.Now()
	tenantID = NormalizeTenantID(tenantID)
	if accessTTL <= 0 {
		accessTTL = time.Duration(cfg.AccessTokenExpire) * time.Second
	}

	accessClaims := Claims{
		UserID:        userID,
		Username:      username,
		TenantID:      tenantID,
		PlatformAdmin: platformAdmin,
		TokenType:     AccessTokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	accessToken, err = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, accessClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	refreshClaims := Claims{
		UserID:        userID,
		Username:      username,
		TenantID:      tenantID,
		PlatformAdmin: platformAdmin,
		TokenType:     RefreshTokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Duration(cfg.RefreshTokenExpire) * time.Second)),
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	refreshToken, err = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, refreshClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func GenerateTOTPChallenge(userID uint, username string, ttl time.Duration) (string, error) {
	return GenerateTOTPChallengeWithTenant(userID, username, 1, ttl)
}

func GenerateTOTPChallengeWithTenant(userID uint, username string, tenantID uint, ttl time.Duration) (string, error) {
	return generateSinglePurposeToken(userID, username, tenantID, TOTPChallengeTokenType, ttl, 5*time.Minute)
}

func GenerateWebSocketTicket(userID uint, username string, ttl time.Duration) (string, error) {
	return generateSinglePurposeToken(userID, username, 1, WebSocketTicketTokenType, ttl, time.Minute)
}

func generateSinglePurposeToken(userID uint, username string, tenantID uint, tokenType string, ttl, defaultTTL time.Duration) (string, error) {
	cfg := config.Cfg.JWT
	now := time.Now()
	tenantID = NormalizeTenantID(tenantID)
	if ttl <= 0 {
		ttl = defaultTTL
	}

	claims := Claims{
		UserID:        userID,
		Username:      username,
		TenantID:      tenantID,
		PlatformAdmin: false,
		TokenType:     tokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	return jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString([]byte(cfg.Secret))
}

func ParseTOTPChallenge(tokenString string) (*Claims, error) {
	claims, err := parseToken(context.Background(), tokenString, true)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TOTPChallengeTokenType {
		return nil, ErrWrongTokenType
	}
	return claims, nil
}

func ParseWebSocketTicket(tokenString string) (*Claims, error) {
	claims, err := parseToken(context.Background(), tokenString, true)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != WebSocketTicketTokenType {
		return nil, ErrWrongTokenType
	}
	return claims, nil
}

func ParseTokenContext(ctx context.Context, tokenString string) (*Claims, error) {
	return parseToken(ctx, tokenString, true)
}

func TokenID(tokenString string) (string, error) {
	claims, err := parseToken(context.Background(), tokenString, false)
	if err != nil || claims.ID == "" {
		return "", ErrInvalidToken
	}
	return claims.ID, nil
}

func parseToken(ctx context.Context, tokenString string, checkRevocation bool) (*Claims, error) {
	cfg := config.Cfg.JWT
	if ctx == nil {
		ctx = context.Background()
	}

	token, err := jwtlib.ParseWithClaims(tokenString, &Claims{}, func(token *jwtlib.Token) (any, error) {
		if _, ok := token.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Secret), nil
	})
	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) || err.Error() == "token is expired" {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if checkRevocation && IsTokenIDBlacklistedContext(ctx, claims.ID) {
		return nil, ErrRevokedToken
	}
	return claims, nil
}

func RefreshToken(refreshToken string) (accessToken, newRefreshToken string, err error) {
	return refreshTokenWithBase(context.Background(), refreshToken)
}

func RefreshTokenContext(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	return refreshTokenWithBase(ctx, refreshToken)
}

func refreshTokenWithBase(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	cfg := config.Cfg.JWT
	if ctx == nil {
		ctx = context.Background()
	}

	claims, err := parseToken(ctx, refreshToken, true)
	if err != nil {
		return "", "", err
	}
	if claims.TokenType != RefreshTokenType {
		return "", "", ErrWrongTokenType
	}

	now := time.Now()
	newAccessClaims := Claims{
		UserID:        claims.UserID,
		Username:      claims.Username,
		TenantID:      NormalizeTenantID(claims.TenantID),
		PlatformAdmin: claims.PlatformAdmin,
		TokenType:     AccessTokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Duration(cfg.AccessTokenExpire) * time.Second)),
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", claims.UserID),
			ID:        uuid.NewString(),
		},
	}

	accessToken, err = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, newAccessClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	if !cfg.RefreshTokenRotation {
		return accessToken, refreshToken, nil
	}

	now = time.Now()
	newRefreshClaims := Claims{
		UserID:        claims.UserID,
		Username:      claims.Username,
		TenantID:      NormalizeTenantID(claims.TenantID),
		PlatformAdmin: claims.PlatformAdmin,
		TokenType:     RefreshTokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Duration(cfg.RefreshTokenExpire) * time.Second)),
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", claims.UserID),
			ID:        uuid.NewString(),
		},
	}

	newRefreshToken, err = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, newRefreshClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	if err := RevokeTokenContext(ctx, refreshToken, claims); err != nil {
		return "", "", err
	}

	return accessToken, newRefreshToken, nil
}

func BlacklistToken(tokenString string, expireTime time.Duration) error {
	return blacklistTokenWithBase(context.Background(), tokenString, expireTime)
}

func BlacklistTokenContext(ctx context.Context, tokenString string, expireTime time.Duration) error {
	return blacklistTokenWithBase(ctx, tokenString, expireTime)
}

func blacklistTokenWithBase(ctx context.Context, tokenString string, expireTime time.Duration) error {
	claims, err := parseToken(ctx, tokenString, false)
	if err != nil || claims.ID == "" {
		return ErrInvalidToken
	}
	return blacklistTokenIDWithBase(ctx, claims.ID, expireTime)
}

func BlacklistTokenID(tokenID string, expireTime time.Duration) error {
	return blacklistTokenIDWithBase(context.Background(), tokenID, expireTime)
}

func BlacklistTokenIDContext(ctx context.Context, tokenID string, expireTime time.Duration) error {
	return blacklistTokenIDWithBase(ctx, tokenID, expireTime)
}

func blacklistTokenIDWithBase(ctx context.Context, tokenID string, expireTime time.Duration) error {
	if tokenID == "" {
		return ErrInvalidToken
	}
	if expireTime <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return currentTokenBlacklistStore().SetTokenID(ctx, tokenID, expireTime)
}

func ConsumeTokenID(ctx context.Context, tokenID string, expireTime time.Duration) (bool, error) {
	if tokenID == "" {
		return false, ErrInvalidToken
	}
	if expireTime <= 0 {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	store := currentTokenBlacklistStore()
	if consumer, ok := store.(tokenIDConsumer); ok {
		return consumer.ConsumeTokenID(ctx, tokenID, expireTime)
	}

	used, err := store.HasTokenID(ctx, tokenID)
	if err != nil {
		return false, err
	}
	if used {
		return false, nil
	}
	if err := store.SetTokenID(ctx, tokenID, expireTime); err != nil {
		return false, err
	}
	return true, nil
}

func RevokeToken(tokenString string, claims *Claims) error {
	return revokeTokenWithBase(context.Background(), tokenString, claims)
}

func RevokeTokenContext(ctx context.Context, tokenString string, claims *Claims) error {
	return revokeTokenWithBase(ctx, tokenString, claims)
}

func revokeTokenWithBase(ctx context.Context, tokenString string, claims *Claims) error {
	if claims == nil || claims.ExpiresAt == nil {
		return ErrInvalidToken
	}
	if ctx == nil {
		ctx = context.Background()
	}
	expireTime := time.Until(claims.ExpiresAt.Time)
	if expireTime <= 0 {
		return nil
	}
	tokenID := claims.ID
	if tokenID == "" {
		parsed, err := parseToken(ctx, tokenString, false)
		if err != nil || parsed.ID == "" {
			return ErrInvalidToken
		}
		tokenID = parsed.ID
	}
	return blacklistTokenIDWithBase(ctx, tokenID, expireTime)
}

func IsTokenBlacklistedContext(ctx context.Context, tokenString string) bool {
	claims, err := parseToken(ctx, tokenString, false)
	if err != nil || claims.ID == "" {
		return false
	}
	return IsTokenIDBlacklistedContext(ctx, claims.ID)
}

func IsTokenIDBlacklistedContext(ctx context.Context, tokenID string) bool {
	if tokenID == "" {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ok, err := currentTokenBlacklistStore().HasTokenID(ctx, tokenID)
	return err == nil && ok
}

func blacklistKey(tokenID string) string {
	return fmt.Sprintf("jwt:blacklist:%s", tokenID)
}

func currentTokenBlacklistStore() TokenBlacklistStore {
	tokenBlacklistStoreMu.RLock()
	store := tokenBlacklistStore
	tokenBlacklistStoreMu.RUnlock()
	if store == nil {
		return redisTokenBlacklistStore{}
	}
	return store
}

func (redisTokenBlacklistStore) SetTokenID(ctx context.Context, tokenID string, expireTime time.Duration) error {
	if redis.Client == nil {
		return errors.New("redis client is not configured")
	}
	return redis.Client.Set(ctx, blacklistKey(tokenID), "1", expireTime).Err()
}

func (redisTokenBlacklistStore) HasTokenID(ctx context.Context, tokenID string) (bool, error) {
	if redis.Client == nil {
		return false, nil
	}
	result, err := redis.Client.Get(ctx, blacklistKey(tokenID)).Result()
	return err == nil && result == "1", nil
}

func (redisTokenBlacklistStore) ConsumeTokenID(ctx context.Context, tokenID string, expireTime time.Duration) (bool, error) {
	if redis.Client == nil {
		return false, errors.New("redis client is not configured")
	}
	return redis.Client.SetNX(ctx, blacklistKey(tokenID), "1", expireTime).Result()
}
