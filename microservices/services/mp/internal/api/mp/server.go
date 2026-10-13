// Package api wires mp-service (微信公众号) HTTP handlers. 后台管理接口与 cc/crm
// 同约定：网关 auth-verify 注入的 X-Auth-* 头优先，Bearer JWT 兜底。微信回调
// （callback）走 mp-open router 绕过网关鉴权，自己做微信签名校验。
package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/aiclient"
	"github.com/go-admin-kit/services/mp/internal/authjwt"
	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
	"gorm.io/gorm"
)

type Server struct {
	Store  *store.Store
	Secret string
	// WX 为微信 API 客户端（M2 素材 + M4 客服/菜单/授权/粉丝 + M5 草稿/发布）。可空
	// （未配 Redis 或本地测试时），需微信主动接口的 handler 会判空降级返回明确错误。
	WX *wxclient.Client
	// AI 调 ai-service 生成文章/封面（M5）。可空。
	AI *aiclient.Client
	// Secrets 解析 appsecret（网页授权换 openid 用）；AESKeyResolver 解析
	// EncodingAESKey（安全模式回调用）。均可空。
	Secrets       wxclient.SecretResolver
	AESKeyForAcct func(acc *store.MpAccount) string
	// Content 内容中台（社媒重构阶段A引入）：平台无关的 Post/发布记录。可空。
	Content *content.Store
	// Publishers 渠道 adapter 注册表（阶段B 起注册 wechat 等）。可空。
	Publishers *publisher.Registry
}

// ---- envelope（统一 {code,message,data}，与核心服务一致）----

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

// notFoundOr 把 gorm 记录未找到映射成 404，其余当 400。
func notFoundOr(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, msg)
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}

// ---- health ----

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "mp-service", "time": time.Now().Format(time.RFC3339)})
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

// ---- auth（后台接口）----

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

// pathID 解析路径参数为正整数 id（照 crm 约定）。
func pathID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// pageOf 从 query 取分页（默认 page=1,page_size=20），越界由 store 层 clamp。
func pageOf(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return page, pageSize
}

// queryUint64 解析可选的数字型 query 参数（缺省/非法返回 0）。
func queryUint64(c *gin.Context, name string) uint64 {
	v, _ := strconv.ParseUint(c.Query(name), 10, 64)
	return v
}
