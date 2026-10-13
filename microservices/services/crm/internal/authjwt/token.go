// Package authjwt validates auth-service access tokens for CRM endpoints,
// mirroring cc/im conventions (gateway headers preferred, JWT fallback).
package authjwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

type AgentClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	TenantID uint64 `json:"tenant_id"`
	// PlatformAdmin 由网关 X-Auth-Platform-Admin 头带入（JWT 兜底路径无此位）。
	PlatformAdmin bool `json:"-"`
	jwt.RegisteredClaims
}

// NormalizeTenantID maps zero/empty tenant to default tenant (id=1).
func NormalizeTenantID(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

// ParseAgent parses auth-service access tokens (user_id claim).
func ParseAgent(secret, token string) (*AgentClaims, error) {
	t, err := jwt.ParseWithClaims(token, &AgentClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := t.Claims.(*AgentClaims)
	if !ok || !t.Valid || c.UserID == 0 {
		return nil, errors.New("invalid agent token")
	}
	c.TenantID = NormalizeTenantID(c.TenantID)
	return c, nil
}
