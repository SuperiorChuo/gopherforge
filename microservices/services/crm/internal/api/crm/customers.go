package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"net/http"
	"strconv"
)

func (s *Server) ListCustomers(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	owner, _ := strconv.ParseUint(c.Query("owner_user_id"), 10, 64)
	list, total, err := s.Store.ListCustomerCards(u.TenantID, store.CustomerFilter{
		Keyword: c.Query("keyword"), Level: c.Query("level"), Owner: owner,
		Scope: c.Query("scope"), Me: u.UserID,
		Access: s.scopeOf(c, u), Page: pageOf(c),
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": list, "total": total})
}

type customerReq struct {
	Name     string `json:"name"`
	Industry string `json:"industry"`
	Level    string `json:"level"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	Note     string `json:"note"`
	Owner    uint64 `json:"owner_user_id"`
}

func (s *Server) CreateCustomer(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req customerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	cu := model.Customer{
		TenantID: u.TenantID, Name: req.Name, Industry: req.Industry,
		Level: defaultStr(req.Level, "C"), Phone: req.Phone, Address: req.Address,
		Note: req.Note, Source: "manual", OwnerUserID: firstU64(req.Owner, u.UserID),
	}
	if err := s.Store.CreateCustomer(c.Request.Context(), &cu); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, cu)
}

func (s *Server) UpdateCustomer(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	cu, err := s.Store.GetCustomer(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "客户不存在")
		return
	}
	if !s.scopeOf(c, u).CanTouchOwner(cu.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权操作该客户")
		return
	}
	var req customerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != "" {
		cu.Name = req.Name
	}
	cu.Industry, cu.Phone, cu.Address, cu.Note = req.Industry, req.Phone, req.Address, req.Note
	if req.Level != "" {
		cu.Level = req.Level
	}
	if req.Owner > 0 {
		cu.OwnerUserID = req.Owner
	}
	if err := s.Store.UpdateCustomer(c.Request.Context(), cu); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, cu)
}

func (s *Server) GetCustomerDetail(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	d, err := s.Store.GetCustomerDetail(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "客户不存在")
		return
	}
	if !s.scopeOf(c, u).CanTouchOwner(d.Customer.OwnerUserID, u.UserID) {
		response.Error(c, http.StatusForbidden, "无权查看该客户")
		return
	}
	response.Success(c, d)
}

type contactReq struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	IsPrimary bool   `json:"is_primary"`
}

func (s *Server) CreateContact(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req contactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	ct := model.Contact{
		TenantID: u.TenantID, CustomerID: id, Name: req.Name, Title: req.Title,
		Phone: req.Phone, Email: req.Email, IsPrimary: req.IsPrimary,
	}
	if err := s.Store.CreateContact(c.Request.Context(), &ct); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, ct)
}

func (s *Server) DeleteContact(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	if err := s.Store.DeleteContact(c.Request.Context(), id, u.TenantID); err != nil {
		notFoundOr(c, err, "联系人不存在")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
