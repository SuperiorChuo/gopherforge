// Package api wires crm-service HTTP handlers. 鉴权与 cc 同约定：网关
// auth-verify 注入的 X-Auth-* 头优先，Bearer JWT 兜底；全部接口须登录。
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/authjwt"
	"github.com/go-admin-kit/services/crm/internal/bpmclient"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type Server struct {
	Store  *store.Store
	Secret string
	// InternalToken 校验 cc-service / bpm 回调等内网服务的 X-Internal-Token
	//（号码识客、挂机写跟进、审批终态回写）。空=禁用内部端点（返回 503），不裸奔。
	InternalToken string
	// BPMEnable 合同审批流开关（BPM_ENABLE）：false 时提交审批/进度端点 503，
	// 其余合同行为与接入前完全一致。
	BPMEnable bool
	// BPM 调 bpm-service internal API 的客户端；nil 或未配 token 视为未启用。
	BPM *bpmclient.Client
	// ContractFlowKey 合同审批使用的流程定义 key（BPM_CONTRACT_FLOW_KEY）。
	ContractFlowKey string
	// PaymentFlowKey 回款审批使用的流程定义 key（BPM_PAYMENT_FLOW_KEY，M2）。
	PaymentFlowKey string
}

// ---- health ----

func (s *Server) HealthLive(c *gin.Context) {
	response.Success(c, gin.H{"status": "alive", "service": "crm-service", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) HealthReady(c *gin.Context) {
	sqlDB, err := s.Store.DB().DB()
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	response.Success(c, gin.H{"status": "ready"})
}

// ---- auth ----

func (s *Server) requireUser(c *gin.Context) (*authjwt.AgentClaims, bool) {
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		if id, err := strconv.ParseUint(uid, 10, 64); err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:        id,
				Username:      c.GetHeader("X-Auth-Username"),
				TenantID:      tenantFromHeader(c),
				PlatformAdmin: c.GetHeader("X-Auth-Platform-Admin") == "1",
			}, true
		}
	}
	tok := bearer(c)
	if tok == "" {
		response.Error(c, http.StatusUnauthorized, "missing token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return claims, true
}

// scopeOf 解析当前用户的 CRM 数据权限（带 5 分钟缓存）。
func (s *Server) scopeOf(c *gin.Context, u *authjwt.AgentClaims) store.UserScope {
	return s.Store.ResolveScope(c.Request.Context(), u.TenantID, u.UserID, u.PlatformAdmin)
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
		response.Error(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func pageOf(c *gin.Context) store.Page {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return store.Page{Page: p, PageSize: ps}
}

// ---- tags (内联，不单独建文件) ----

// 标签与其他 CRM 接口共用网关注入的租户头；真实路由不会填充 Gin 的 tenant_id key。
func (s *Server) ListTags(c *gin.Context) {
	tags, err := s.Store.TagDAO().List(c.Request.Context(), tenantFromHeader(c))
	if err != nil {
		response.InternalServerError(c, "failed to list tags")
		return
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	response.Success(c, tags)
}

func (s *Server) CreateTag(c *gin.Context) {
	var t model.Tag
	if err := c.ShouldBindJSON(&t); err != nil || t.Name == "" {
		response.BadRequest(c, "name required")
		return
	}
	t.TenantID = tenantFromHeader(c)
	if err := s.Store.TagDAO().Create(c.Request.Context(), &t); err != nil {
		response.InternalServerError(c, "create failed")
		return
	}
	response.Success(c, t)
}

func (s *Server) UpdateTag(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var t model.Tag
	if err := c.ShouldBindJSON(&t); err != nil {
		response.BadRequest(c, "invalid")
		return
	}
	t.ID = id
	t.TenantID = tenantFromHeader(c)
	if err := s.Store.TagDAO().Update(c.Request.Context(), &t); err != nil {
		response.InternalServerError(c, "update failed")
		return
	}
	response.Success(c, t)
}

func (s *Server) DeleteTag(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.Store.TagDAO().Delete(c.Request.Context(), id, tenantFromHeader(c)); err != nil {
		response.InternalServerError(c, "delete failed")
		return
	}
	response.Success(c, nil)
}
