package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
)

func (s *Server) ListOpportunities(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	customerID, _ := strconv.ParseUint(c.Query("customer_id"), 10, 64)
	owner, _ := strconv.ParseUint(c.Query("owner_user_id"), 10, 64)
	list, total, err := s.Store.ListOpportunities(u.TenantID, store.OppFilter{
		Stage: c.Query("stage"), CustomerID: customerID, Owner: owner,
		Access: s.scopeOf(c, u), Page: pageOf(c),
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list, "total": total})
}

type oppReq struct {
	CustomerID      uint64  `json:"customer_id"`
	Name            string  `json:"name"`
	AmountCents     int64   `json:"amount_cents"`
	Stage           string  `json:"stage"`
	ExpectedCloseAt *string `json:"expected_close_at"` // "2026-08-31"
	Note            string  `json:"note"`
	Owner           uint64  `json:"owner_user_id"`
}

func (s *Server) CreateOpportunity(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req oppReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CustomerID == 0 {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	o := model.Opportunity{
		TenantID: u.TenantID, CustomerID: req.CustomerID, Name: req.Name,
		AmountCents: req.AmountCents, Stage: defaultStr(req.Stage, "initial"),
		ExpectedCloseAt: parseDate(req.ExpectedCloseAt), Note: req.Note,
		OwnerUserID: firstU64(req.Owner, u.UserID),
	}
	if err := s.Store.CreateOpportunity(&o, u.UserID); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, o)
}

func (s *Server) UpdateOpportunity(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	o, err := s.Store.GetOpportunity(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "商机不存在")
		return
	}
	sc := s.scopeOf(c, u)
	if !sc.CanTouchOwner(o.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权操作该商机")
		return
	}
	var req oppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != "" {
		o.Name = req.Name
	}
	if req.AmountCents > 0 {
		o.AmountCents = req.AmountCents
	}
	if d := parseDate(req.ExpectedCloseAt); d != nil {
		o.ExpectedCloseAt = d
	}
	o.Note = req.Note
	if req.Owner > 0 {
		// 转移负责人：新负责人也须在自己可见范围内（self=只能给自己，
		// dept=树内成员，all=不限），防转给任意用户越权注水
		if !sc.CanTouchOwner(req.Owner, u.UserID) {
			response.Error(c, http.StatusForbidden, "无权将商机转移给该成员")
			return
		}
		o.OwnerUserID = req.Owner
	}
	if err := s.Store.UpdateOpportunity(o); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, o)
}

func (s *Server) GetOpportunity(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	o, err := s.Store.GetOpportunity(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "商机不存在")
		return
	}
	if !s.scopeOf(c, u).CanTouchOwner(o.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权查看该商机")
		return
	}
	logs, _ := s.Store.ListStageLogs(o.ID)
	response.Success(c, gin.H{"opportunity": o, "stage_logs": logs})
}

type stageReq struct {
	Stage      string `json:"stage"`
	Note       string `json:"note"`
	LostReason string `json:"lost_reason"`
}

func (s *Server) AdvanceStage(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req stageReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Stage == "" {
		response.Error(c, http.StatusBadRequest, "stage is required")
		return
	}
	cur, err := s.Store.GetOpportunity(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "商机不存在")
		return
	}
	if !s.scopeOf(c, u).CanTouchOwner(cur.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权操作该商机")
		return
	}
	o, err := s.Store.AdvanceStage(id, u.TenantID, req.Stage, req.Note, req.LostReason, u.UserID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, o)
}
