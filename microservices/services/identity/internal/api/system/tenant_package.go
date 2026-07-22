package system

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// TenantPackageAPI 租户套餐（权限包）CRUD，仅平台运营侧使用。
type TenantPackageAPI struct {
	svc *systemsvc.TenantPackageService
}

func NewTenantPackageAPIWithService(svc *systemsvc.TenantPackageService) *TenantPackageAPI {
	return &TenantPackageAPI{svc: svc}
}

func writeTenantPackageServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrTenantPackageNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, systemsvc.ErrTenantPackageNameRequired),
		errors.Is(err, systemsvc.ErrTenantPackageNameExists),
		errors.Is(err, systemsvc.ErrTenantPackageInUse):
		response.BadRequest(c, err.Error())
	default:
		internalServerError(c, operation, err)
	}
}

func (a *TenantPackageAPI) GetTenantPackageList(c *gin.Context) {
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
		if v, err := strconv.ParseInt(s, 10, 8); err == nil {
			sv := int8(v)
			status = &sv
		}
	}
	list, total, err := a.svc.List(c.Request.Context(), systemsvc.TenantPackageListRequest{
		PageRequest: page,
		Keyword:     c.Query("keyword"),
		Status:      status,
	})
	if err != nil {
		internalServerError(c, "failed to get tenant package list", err)
		return
	}
	response.PageSuccess(c, list, total, page.Page, page.PageSize)
}

// GetAllTenantPackages 全量套餐（租户管理页下拉选择用）。
func (a *TenantPackageAPI) GetAllTenantPackages(c *gin.Context) {
	list, err := a.svc.GetAll(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get tenant packages", err)
		return
	}
	response.Success(c, list)
}

func (a *TenantPackageAPI) GetTenantPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	p, err := a.svc.Get(c.Request.Context(), uint(id))
	if err != nil {
		writeTenantPackageServiceError(c, "failed to get tenant package", err)
		return
	}
	response.Success(c, p)
}

func (a *TenantPackageAPI) CreateTenantPackage(c *gin.Context) {
	var req systemsvc.CreateTenantPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	p, err := a.svc.Create(c.Request.Context(), req)
	if err != nil {
		writeTenantPackageServiceError(c, "failed to create tenant package", err)
		return
	}
	response.Success(c, p)
}

func (a *TenantPackageAPI) UpdateTenantPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	var req systemsvc.UpdateTenantPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	p, err := a.svc.Update(c.Request.Context(), uint(id), req)
	if err != nil {
		writeTenantPackageServiceError(c, "failed to update tenant package", err)
		return
	}
	response.Success(c, p)
}

func (a *TenantPackageAPI) DeleteTenantPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := a.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		writeTenantPackageServiceError(c, "failed to delete tenant package", err)
		return
	}
	response.SuccessWithMessage(c, "tenant package deleted successfully", nil)
}
