package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// 自定义错误
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

// Claims 自定义JWT声明
type Claims struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// GenerateToken 生成AccessToken和RefreshToken
func GenerateToken(userID uint, username string) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	return GenerateTokenWithAccessTTL(userID, username, time.Duration(cfg.AccessTokenExpire)*time.Second)
}

// GenerateTokenWithAccessTTL generates access/refresh tokens with a custom access-token TTL.
func GenerateTokenWithAccessTTL(userID uint, username string, accessTTL time.Duration) (accessToken, refreshToken string, err error) {
	cfg := config.Cfg.JWT
	now := time.Now()
	if accessTTL <= 0 {
		accessTTL = time.Duration(cfg.AccessTokenExpire) * time.Second
	}

	// 生成AccessToken
	accessClaims := Claims{
		UserID:    userID,
		Username:  username,
		TokenType: AccessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	// 生成RefreshToken
	refreshClaims := Claims{
		UserID:    userID,
		Username:  username,
		TokenType: RefreshTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(cfg.RefreshTokenExpire) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	refreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// ParseToken 解析Token
func ParseToken(tokenString string) (*Claims, error) {
	cfg := config.Cfg.JWT

	// 解析token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Secret), nil
	})

	if err != nil {
		// 处理过期错误
		if err.Error() == "token is expired" {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	// 验证token是否有效
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// 检查token是否在黑名单中
		if IsTokenBlacklisted(tokenString) {
			return nil, ErrRevokedToken
		}
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// RefreshToken 轮换RefreshToken并签发新的AccessToken。
func RefreshToken(refreshToken string) (accessToken, newRefreshToken string, err error) {
	cfg := config.Cfg.JWT

	// 解析refreshToken
	claims, err := ParseToken(refreshToken)
	if err != nil {
		return "", "", err
	}
	if claims.TokenType != RefreshTokenType {
		return "", "", ErrWrongTokenType
	}

	// 生成新的AccessToken
	newAccessClaims := Claims{
		UserID:    claims.UserID,
		Username:  claims.Username,
		TokenType: AccessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.AccessTokenExpire) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", claims.UserID),
			ID:        uuid.NewString(),
		},
	}

	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, newAccessClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	if !cfg.RefreshTokenRotation {
		return accessToken, refreshToken, nil
	}

	newRefreshClaims := Claims{
		UserID:    claims.UserID,
		Username:  claims.Username,
		TokenType: RefreshTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.RefreshTokenExpire) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    cfg.Issuer,
			Subject:   fmt.Sprintf("%d", claims.UserID),
			ID:        uuid.NewString(),
		},
	}

	newRefreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, newRefreshClaims).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", "", err
	}

	if err := RevokeToken(refreshToken, claims); err != nil {
		return "", "", err
	}

	return accessToken, newRefreshToken, nil
}

// BlacklistToken 将Token加入黑名单
func BlacklistToken(tokenString string, expireTime time.Duration) error {
	ctx := context.Background()
	return redis.Client.Set(ctx, fmt.Sprintf("jwt:blacklist:%s", tokenString), "1", expireTime).Err()
}

// RevokeToken 将未过期Token加入黑名单。
func RevokeToken(tokenString string, claims *Claims) error {
	if claims == nil || claims.ExpiresAt == nil {
		return ErrInvalidToken
	}
	expireTime := time.Until(claims.ExpiresAt.Time)
	if expireTime <= 0 {
		return nil
	}
	return BlacklistToken(tokenString, expireTime)
}

// IsTokenBlacklisted 检查Token是否在黑名单中
func IsTokenBlacklisted(tokenString string) bool {
	ctx := context.Background()
	result, err := redis.Client.Get(ctx, fmt.Sprintf("jwt:blacklist:%s", tokenString)).Result()
	return err == nil && result == "1"
}
