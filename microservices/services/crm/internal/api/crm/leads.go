package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
)

func (s *Server) ListLeads(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	owner, _ := strconv.ParseUint(c.Query("owner_user_id"), 10, 64)
	deptID, _ := strconv.ParseUint(c.Query("department_id"), 10, 64)
	list, total, err := s.Store.ListLeads(u.TenantID, store.LeadFilter{
		Status: c.Query("status"), Keyword: c.Query("keyword"), Owner: owner,
		Scope: c.Query("scope"), DeptID: deptID, Me: u.UserID,
		Access: s.scopeOf(c, u), Page: pageOf(c),
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list, "total": total})
}

type leadReq struct {
	Name        string `json:"name"`
	Company     string `json:"company"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Source      string `json:"source"`
	Status      string `json:"status"`
	Note        string `json:"note"`
	Owner       uint64 `json:"owner_user_id"`
	IMVisitorID uint64 `json:"im_visitor_id"`
}

func (s *Server) CreateLead(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req leadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	l := model.Lead{
		TenantID: u.TenantID, Name: req.Name, Company: req.Company,
		Phone: req.Phone, Email: req.Email, Note: req.Note,
		Source: defaultStr(req.Source, "manual"), Status: defaultStr(req.Status, "new"),
		OwnerUserID: firstU64(req.Owner, u.UserID),
		IMVisitorID: req.IMVisitorID,
	}
	if err := s.Store.CreateLead(c.Request.Context(), &l); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, l)
}

func (s *Server) UpdateLead(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	l, err := s.Store.GetLead(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "线索不存在")
		return
	}
	if !s.scopeOf(c, u).CanTouchOwner(l.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权操作该名片")
		return
	}
	var req leadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != "" {
		l.Name = req.Name
	}
	l.Company, l.Phone, l.Email, l.Note = req.Company, req.Phone, req.Email, req.Note
	if req.Status != "" && l.Status != "converted" {
		l.Status = req.Status
	}
	if req.Owner > 0 {
		l.OwnerUserID = req.Owner
	}
	if err := s.Store.UpdateLead(c.Request.Context(), l); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, l)
}

type convertReq struct {
	OpportunityName string `json:"opportunity_name"`
	AmountCents     int64  `json:"amount_cents"`
}

func (s *Server) ConvertLead(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req convertReq
	_ = c.ShouldBindJSON(&req)
	customer, opp, err := s.Store.ConvertLead(id, u.TenantID, u.UserID, req.OpportunityName, req.AmountCents)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"customer": customer, "opportunity": opp})
}
