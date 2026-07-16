package system

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/response"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
)

// TenantAPI exposes tenant CRUD for platform operators.
type TenantAPI struct {
	svc *systemsvc.TenantService
}

func NewTenantAPIWithService(svc *systemsvc.TenantService) *TenantAPI {
	return &TenantAPI{svc: svc}
}

func (a *TenantAPI) GetTenantList(c *gin.Context) {
	var page pagination.PageRequest
	_ = c.ShouldBindQuery(&page)
	if page.Page <= 0 {
		page.Page = 1
	}
	if page.PageSize <= 0 {
		page.PageSize = 10
	}
	var status *int8
	if s := c.Query("status"); s != "" {
		v, err := strconv.ParseInt(s, 10, 8)
		if err == nil {
			sv := int8(v)
			status = &sv
		}
	}
	list, total, err := a.svc.List(c.Request.Context(), systemsvc.TenantListRequest{
		PageRequest: page,
		Keyword:     c.Query("keyword"),
		Status:      status,
	})
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	response.PageSuccess(c, list, total, page.Page, page.PageSize)
}

func (a *TenantAPI) GetTenant(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	t, err := a.svc.Get(c.Request.Context(), uint(id))
	if err != nil {
		if errors.Is(err, systemsvc.ErrTenantNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalServerError(c, err.Error())
		return
	}
	count, _ := a.svc.UserCount(c.Request.Context(), t.ID)
	response.Success(c, gin.H{"tenant": t, "user_count": count})
}

func (a *TenantAPI) CreateTenant(c *gin.Context) {
	var req systemsvc.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	t, err := a.svc.Create(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, systemsvc.ErrTenantCodeExists),
			errors.Is(err, systemsvc.ErrTenantCodeInvalid),
			errors.Is(err, systemsvc.ErrTenantNameRequired):
			response.BadRequest(c, err.Error())
		default:
			response.InternalServerError(c, err.Error())
		}
		return
	}
	response.Success(c, t)
}

func (a *TenantAPI) UpdateTenant(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	var req systemsvc.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	t, err := a.svc.Update(c.Request.Context(), uint(id), req)
	if err != nil {
		if errors.Is(err, systemsvc.ErrTenantNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		if errors.Is(err, systemsvc.ErrTenantNameRequired) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, t)
}
