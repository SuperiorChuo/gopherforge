package api

// P1 名片分配 + P2 公海规则引擎的 HTTP 层。

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"

	"github.com/go-admin-kit/services/shared/pkg/response"
)

// ---------- 组织数据 ----------

// Org handles GET /api/v1/crm/org — 分配选择器数据（部门+成员，仅本租户）。
func (s *Server) Org(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	depts, users := s.Store.Org(c.Request.Context(), u.TenantID)
	response.Success(c, gin.H{"departments": depts, "users": users})
}

// ---------- 名片分配 ----------

type assignLeadsReq struct {
	IDs          []uint64 `json:"ids"`
	DepartmentID uint64   `json:"department_id"`
	OwnerUserID  uint64   `json:"owner_user_id"`
}

// AssignLeads handles POST /api/v1/crm/leads/assign.
func (s *Server) AssignLeads(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req assignLeadsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	n, err := s.Store.AssignLeads(c.Request.Context(), u.TenantID, req.IDs, req.DepartmentID, req.OwnerUserID, u.UserID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"assigned": n})
}

// ClaimLead handles POST /api/v1/crm/leads/:id/claim.
func (s *Server) ClaimLead(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	l, err := s.Store.ClaimLead(id, u.TenantID, u.UserID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, l)
}

// ---------- 公海规则引擎 ----------

// GetPoolRules handles GET /api/v1/crm/pool/rules.
func (s *Server) GetPoolRules(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	rules, err := s.Store.GetPoolRules(u.TenantID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"rules": rules})
}

type poolRuleReq struct {
	Level         string `json:"level"`
	IdleDays      int    `json:"idle_days"`
	ProtectActive bool   `json:"protect_active"`
	Enabled       bool   `json:"enabled"`
}

// SavePoolRule handles PUT /api/v1/crm/pool/rules.
func (s *Server) SavePoolRule(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req poolRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	r := model.PoolRule{
		TenantID: u.TenantID, Level: req.Level, IdleDays: req.IdleDays,
		ProtectActive: req.ProtectActive, Enabled: req.Enabled, UpdatedBy: u.UserID,
	}
	if err := s.Store.SavePoolRule(&r); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, r)
}

// MyScope handles GET /api/v1/crm/my-scope — 前端按档位收敛视图
// （self 档隐藏「全部」等）。
func (s *Server) MyScope(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	sc := s.scopeOf(c, u)
	response.Success(c, gin.H{"scope": string(sc.Kind), "visible_users": len(sc.VisibleUserIDs)})
}

// GetPoolStats handles GET /api/v1/crm/pool/stats — 公海规则页实时态。
func (s *Server) GetPoolStats(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	stats, err := s.Store.PoolStatsFor(u.TenantID, 20)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, stats)
}

// RecycleNow handles POST /api/v1/crm/pool/recycle — 手动触发一次回收
// （规则页"立即执行"，无需等每小时扫描）。
//
// 只在请求里跑只读的计划阶段（3 条查询），回池写入与留痕交给后台 goroutine，
// 接口即刻返回本轮受理条数；前端拿到条数后刷新实时态即可看到落库结果。
func (s *Server) RecycleNow(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	plan, err := s.Store.PlanRecycle(u.TenantID, 200)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]uint64, 0, len(plan))
	for _, r := range plan {
		ids = append(ids, r.ID)
	}
	if len(plan) > 0 {
		tenantID := u.TenantID
		go func() {
			if _, err := s.Store.ApplyRecycle(tenantID, plan); err != nil {
				log.Printf("crm pool: tenant %d apply recycle: %v", tenantID, err)
			}
		}()
	}
	response.Success(c, gin.H{"recycled": len(plan), "ids": ids, "accepted": true})
}

// ---------- 客户领取/释放 ----------

// ClaimCustomer handles POST /api/v1/crm/customers/:id/claim.
func (s *Server) ClaimCustomer(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	cu, err := s.Store.ClaimCustomer(id, u.TenantID, u.UserID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, cu)
}

type releaseReq struct {
	Note string `json:"note"`
}

// ReleaseCustomer handles POST /api/v1/crm/customers/:id/release.
func (s *Server) ReleaseCustomer(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req releaseReq
	_ = c.ShouldBindJSON(&req)
	if err := s.Store.ReleaseCustomer(id, u.TenantID, u.UserID, req.Note); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"released": true})
}

// ListAssignLogs handles GET /api/v1/crm/assign-logs?target_type=&target_id=.
func (s *Server) ListAssignLogs(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	targetID, _ := strconv.ParseUint(c.Query("target_id"), 10, 64)
	logs, err := s.Store.ListAssignLogs(u.TenantID, c.Query("target_type"), targetID, 20)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": logs})
}

// ---------- 客户合并去重 ----------

// FindDupGroups handles GET /api/v1/crm/dup-groups — 重复客户检测
// （号码尾 11 位或同名）。结果再按数据权限过滤：组内有越权客户的整组隐藏，
// 避免向 self/dept 档泄露他人客户。
func (s *Server) FindDupGroups(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	groups, err := s.Store.FindDuplicateGroups(u.TenantID, 20)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	sc := s.scopeOf(c, u)
	visible := groups[:0]
	for _, g := range groups {
		okAll := true
		for _, cu := range g.Customers {
			if !sc.CanTouchOwner(cu.OwnerUserID, u.UserID) {
				okAll = false
				break
			}
		}
		if okAll {
			visible = append(visible, g)
		}
	}
	response.Success(c, gin.H{"groups": visible, "total": len(visible)})
}

type mergeReq struct {
	PrimaryID uint64   `json:"primary_id"`
	SourceIDs []uint64 `json:"source_ids"`
}

// MergeCustomers handles POST /api/v1/crm/customers/merge — 事务合并。
func (s *Server) MergeCustomers(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req mergeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PrimaryID == 0 || len(req.SourceIDs) == 0 {
		response.Error(c, http.StatusBadRequest, "primary_id 与 source_ids 必填")
		return
	}
	sourceIDs, err := store.NormalizeMergeSourceIDs(req.PrimaryID, req.SourceIDs)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	// scope 校验全部参与客户
	sc := s.scopeOf(c, u)
	ids := append([]uint64{req.PrimaryID}, sourceIDs...)
	customers, err := s.Store.GetCustomersByIDs(u.TenantID, ids)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if len(customers) != len(ids) {
		response.Error(c, http.StatusNotFound, "客户不存在")
		return
	}
	for _, cu := range customers {
		if !sc.CanTouchOwner(cu.OwnerUserID, u.UserID) {
			response.Error(c, http.StatusForbidden, "无权合并他人客户")
			return
		}
	}
	stats, err := s.Store.MergeCustomers(u.TenantID, req.PrimaryID, sourceIDs, u.UserID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"merged": len(sourceIDs), "moved": stats})
}
