package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/notify/internal/authjwt"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
)

type Server struct {
	Store         *store.Store
	Secret        string
	InternalToken string
	Dispatcher    *channel.Dispatcher
	// Outbox 异步外发队列（email/sms）；nil 时 internal/send 不做外部投递。
	Outbox *Outbox
	// AlertUserID Prometheus 告警站内信收件人（运维管理员），0 时回退 1。
	AlertUserID uint64
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "notify-service", "time": time.Now().Format(time.RFC3339)})
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
	ok(c, gin.H{"status": "ready", "channels": s.channelStatus()})
}

func (s *Server) channelStatus() map[string]any {
	if s.Dispatcher != nil {
		return s.Dispatcher.Status()
	}
	return map[string]any{"inbox": true}
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

func (s *Server) requireInternal(c *gin.Context) bool {
	if s.InternalToken == "" {
		fail(c, http.StatusServiceUnavailable, "internal notify not configured")
		return false
	}
	if c.GetHeader("X-Internal-Token") != s.InternalToken {
		fail(c, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}
