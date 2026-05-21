package jwt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/redis"
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
	AccessTokenType  = "access"
	RefreshTokenType = "refresh"
)

// TokenBlacklistStore stores revoked JWT token IDs.
type TokenBlacklistStore interface {
	SetTokenID(ctx context.Context, tokenID string, expireTime time.Duration) error
	HasTokenID(ctx context.Context, tokenID string) (bool, error)
}

var (
	tokenBlacklistStoreMu sync.RWMutex
	tokenBlacklistStore   TokenBlacklistStore = redisTokenBlacklistStore{}
)

type redisTokenBlacklistStore struct{}

type Claims struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	TokenType string `json:"token_type"`
	jwtlib.RegisteredClaims
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
	return GenerateTokenWithAccessTTL(userID, username, time.Duration(cfg.AccessTokenExpire)*time.Second)
}

func GenerateTokenWithAccessTTL(userID uint, username string, accessTTL time.Duration) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	now := time.Now()
	if accessTTL <= 0 {
		accessTTL = time.Duration(cfg.AccessTokenExpire) * time.Second
	}

	accessClaims := Claims{
		UserID:    userID,
		Username:  username,
		TokenType: AccessTokenType,
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
		UserID:    userID,
		Username:  username,
		TokenType: RefreshTokenType,
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

func ParseToken(tokenString string) (*Claims, error) {
	return parseToken(tokenString, true)
}

func TokenID(tokenString string) (string, error) {
	claims, err := parseToken(tokenString, false)
	if err != nil || claims.ID == "" {
		return "", ErrInvalidToken
	}
	return claims.ID, nil
}

func parseToken(tokenString string, checkRevocation bool) (*Claims, error) {
	cfg := config.Cfg.JWT

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
	if checkRevocation && IsTokenIDBlacklisted(claims.ID) {
		return nil, ErrRevokedToken
	}
	return claims, nil
}

func RefreshToken(refreshToken string) (accessToken, newRefreshToken string, err error) {
	cfg := config.Cfg.JWT

	claims, err := ParseToken(refreshToken)
	if err != nil {
		return "", "", err
	}
	if claims.TokenType != RefreshTokenType {
		return "", "", ErrWrongTokenType
	}

	now := time.Now()
	newAccessClaims := Claims{
		UserID:    claims.UserID,
		Username:  claims.Username,
		TokenType: AccessTokenType,
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
		UserID:    claims.UserID,
		Username:  claims.Username,
		TokenType: RefreshTokenType,
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

	if err := RevokeToken(refreshToken, claims); err != nil {
		return "", "", err
	}

	return accessToken, newRefreshToken, nil
}

func BlacklistToken(tokenString string, expireTime time.Duration) error {
	claims, err := parseToken(tokenString, false)
	if err != nil || claims.ID == "" {
		return ErrInvalidToken
	}
	return BlacklistTokenID(claims.ID, expireTime)
}

func BlacklistTokenID(tokenID string, expireTime time.Duration) error {
	if tokenID == "" {
		return ErrInvalidToken
	}
	if expireTime <= 0 {
		return nil
	}
	ctx := context.Background()
	return currentTokenBlacklistStore().SetTokenID(ctx, tokenID, expireTime)
}

func RevokeToken(tokenString string, claims *Claims) error {
	if claims == nil || claims.ExpiresAt == nil {
		return ErrInvalidToken
	}
	expireTime := time.Until(claims.ExpiresAt.Time)
	if expireTime <= 0 {
		return nil
	}
	tokenID := claims.ID
	if tokenID == "" {
		parsed, err := parseToken(tokenString, false)
		if err != nil || parsed.ID == "" {
			return ErrInvalidToken
		}
		tokenID = parsed.ID
	}
	return BlacklistTokenID(tokenID, expireTime)
}

func IsTokenBlacklisted(tokenString string) bool {
	claims, err := parseToken(tokenString, false)
	if err != nil || claims.ID == "" {
		return false
	}
	return IsTokenIDBlacklisted(claims.ID)
}

func IsTokenIDBlacklisted(tokenID string) bool {
	if tokenID == "" {
		return false
	}
	ctx := context.Background()
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
