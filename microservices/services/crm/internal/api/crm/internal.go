package api

// 服务间内部端点（cc-service → crm-service）：号码识客 + 挂机写跟进。
// 鉴权与 webhook/CDR 先例一致——共享密钥走 X-Internal-Token 头（非 JWT，
// 不走网关 ForwardAuth）；调用方在内网直连 crm 容器 :8091。租户经
// X-Tenant-ID 头传入（缺省 1）。密钥为空时禁用（返回 503），避免误裸奔。

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/authjwt"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// requireInternal 校验 X-Internal-Token；未配置密钥直接 503（拒绝裸奔）。
func (s *Server) requireInternal(c *gin.Context) bool {
	if s.InternalToken == "" {
		response.Error(c, http.StatusServiceUnavailable, "internal endpoint disabled")
		return false
	}
	got := c.GetHeader("X-Internal-Token")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.InternalToken)) != 1 {
		response.Error(c, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}

// internalTenant 从 X-Tenant-ID 头取租户（缺省 1），归一化。
func internalTenant(c *gin.Context) uint64 {
	if v := c.GetHeader("X-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return authjwt.NormalizeTenantID(id)
		}
	}
	return 1
}

// InternalLookup handles GET /api/v1/crm/internal/lookup?phone= — 号码识客
// （cc 来电弹屏聚合的一环）。复用 LookupByPhone，仅换成内部 token 鉴权。
func (s *Server) InternalLookup(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	res, err := s.Store.LookupByPhone(internalTenant(c), c.Query("phone"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, res)
}

type internalFollowReq struct {
	TargetType     string `json:"target_type"` // 目前仅 customer
	TargetID       uint64 `json:"target_id"`
	Content        string `json:"content"`
	ByUserID       uint64 `json:"by_user_id"` // 归因坐席用户；0=系统
	IdempotencyKey string `json:"idempotency_key"`
}

// InternalCreateFollowUp handles POST /api/v1/crm/internal/followups — 挂机
// 后 cc 给命中的 CRM 客户写一条通话跟进。目标必须存在于同租户；
// X-Idempotency-Key / body.idempotency_key 支持 gRPC 回退防双写。
func (s *Server) InternalCreateFollowUp(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req internalFollowReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TargetID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
	}
	f := model.FollowUp{
		TenantID:   internalTenant(c),
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Content:    req.Content,
		ByUserID:   req.ByUserID,
	}
	if err := s.Store.CreateFollowUpIdempotent(c.Request.Context(), &f, key); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, f)
}
