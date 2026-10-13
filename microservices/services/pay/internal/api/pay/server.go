// Package api wires pay-service HTTP handlers.
// Auth: gateway ForwardAuth injects X-Auth-*; Bearer JWT fallback for bare connections.
// Internal endpoints use X-Internal-Token (PAY_INTERNAL_TOKEN; empty disables them).
// Cashier endpoints are anonymous — the order_token itself is the credential.
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/authjwt"
	"github.com/go-admin-kit/services/pay/internal/channel"
	"github.com/go-admin-kit/services/pay/internal/store/pay"
)

type Server struct {
	Store    *store.Store
	Secret   string
	Registry *channel.Registry
	// InternalToken 空 = 禁用 /internal 端点（照 CRM/BPM 先例）
	InternalToken string
	// OrderTTL 建单默认过期时长
	OrderTTL time.Duration
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "pay-service", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) HealthReady(c *gin.Context) {
	sqlDB, err := s.Store.DB().DB()
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ok(c, gin.H{"status": "ready"})
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func tenantFromHeader(c *gin.Context) uint64 {
	if v := c.GetHeader("X-Auth-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return authjwt.NormalizeTenantID(id)
		}
	}
	return 1
}

func (s *Server) requireUser(c *gin.Context) (*authjwt.AgentClaims, bool) {
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		if id, err := strconv.ParseUint(uid, 10, 64); err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:   id,
				Username: c.GetHeader("X-Auth-Username"),
				TenantID: tenantFromHeader(c),
			}, true
		}
	}
	tok := bearer(c)
	if tok == "" {
		fail(c, http.StatusUnauthorized, "missing token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		fail(c, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return claims, true
}

// requireInternal internal 端点鉴权：X-Internal-Token 恒等比较；未配置=禁用。
func (s *Server) requireInternal(c *gin.Context) bool {
	if s.InternalToken == "" {
		fail(c, http.StatusForbidden, "internal endpoints disabled")
		return false
	}
	if c.GetHeader("X-Internal-Token") != s.InternalToken {
		fail(c, http.StatusUnauthorized, "bad internal token")
		return false
	}
	return true
}
