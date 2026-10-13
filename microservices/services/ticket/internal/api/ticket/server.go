// Package api wires ticket-service HTTP handlers.
// Auth: gateway ForwardAuth injects X-Auth-*; Bearer JWT fallback for bare connections.
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/ticket/internal/authjwt"
	"github.com/go-admin-kit/services/ticket/internal/store/ticket"
)

type Server struct {
	Store  *store.Store
	Secret string
	// Notify optional inbox push (指派/超时由 cron 另跑)；nil = disabled.
	Notify *notifyclient.Client
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "ticket-service", "time": time.Now().Format(time.RFC3339)})
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

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
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

func pathID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func pageOf(c *gin.Context) store.Page {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return store.Page{Page: p, PageSize: ps}
}
