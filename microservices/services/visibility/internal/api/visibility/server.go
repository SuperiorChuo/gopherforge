// Package api wires visibility-service HTTP handlers (SEO + GEO).
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/visibility/internal/authjwt"
	"github.com/go-admin-kit/services/visibility/internal/runner"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
)

type Server struct {
	Store     *store.Store
	Runner    *runner.Runner
	Secret    string
	AIBaseURL string
	AIAPIKey  string
	AIModel   string
	// DemoMode 演示假数据总闸（VISIBILITY_DEMO_MODE，生产默认关）。关时：
	// stub 模式检测被拒、排行榜不回落演示数据、演示播种路由不挂载，
	// 前端也据此隐藏「模拟演示」「初始化演示数据」入口（见响应里的 demo_mode）。
	DemoMode bool
}

func writeOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func writeFail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

func (s *Server) HealthLive(c *gin.Context) {
	writeOK(c, gin.H{"status": "alive", "service": "visibility-service", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) HealthReady(c *gin.Context) {
	sqlDB, err := s.Store.DB().DB()
	if err != nil {
		writeFail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	// DB 半开连接时无超时的 Ping 会挂住探针请求并堆积 goroutine
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if sqlDB.PingContext(ctx) != nil {
		writeFail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeOK(c, gin.H{"status": "ready"})
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
		writeFail(c, http.StatusUnauthorized, "missing token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		writeFail(c, http.StatusUnauthorized, "invalid token")
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
		writeFail(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func pageOf(c *gin.Context) store.Page {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return store.Page{Page: p, PageSize: ps}
}
