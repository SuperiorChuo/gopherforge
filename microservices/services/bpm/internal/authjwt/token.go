// Package authjwt 校验 auth-service 签发的访问令牌（网关 X-Auth-* 头优先，
// JWT 兜底裸连场景）。
package authjwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

type AgentClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	TenantID uint64 `json:"tenant_id"`
	// PlatformAdmin 平台管理员（网关 X-Auth-Platform-Admin 头注入），
	// 用于实例可见性放行（bpm:instance:query-all 语义的 M1 从简实现）。
	PlatformAdmin bool `json:"-"`
	jwt.RegisteredClaims
}

func NormalizeTenantID(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

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
