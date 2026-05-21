package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// MenuManagementAPI handles menu management endpoints.
type MenuManagementAPI struct {
	menuService system.MenuService
}

// NewMenuManagementAPI creates a MenuManagementAPI instance.
func NewMenuManagementAPI() *MenuManagementAPI {
	return &MenuManagementAPI{
		menuService: system.MenuService{},
	}
}

// GetMenuList returns paginated menus.
func (a *MenuManagementAPI) GetMenuList(c *gin.Context) {
	var req system.MenuListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(status)
			req.Status = &statusInt8
		}
	}

	menus, total, err := a.menuService.GetMenuListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get menu list", err)
		return
	}

	response.PageSuccess(c, menus, total, req.Page, req.PageSize)
}

// GetMenuTree returns a menu tree.
func (a *MenuManagementAPI) GetMenuTree(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	menus, err := a.menuService.GetMenuTreeContext(c.Request.Context(), status)
	if err != nil {
		internalServerError(c, "failed to get menu tree", err)
		return
	}

	response.Success(c, menus)
}

// GetMenu returns a menu by id.
func (a *MenuManagementAPI) GetMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	menu, err := a.menuService.GetMenuByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemMenuServiceError(c, "failed to get menu", err)
		return
	}

	response.Success(c, menu)
}

// CreateMenu creates a menu.
func (a *MenuManagementAPI) CreateMenu(c *gin.Context) {
	var req system.CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	menu, err := a.menuService.CreateMenuContext(c.Request.Context(), req)
	if err != nil {
		writeSystemMenuServiceError(c, "failed to create menu", err)
		return
	}

	response.SuccessWithMessage(c, "menu created successfully", menu)
}

// UpdateMenu updates a menu.
func (a *MenuManagementAPI) UpdateMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	var req system.UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	menu, err := a.menuService.UpdateMenuContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemMenuServiceError(c, "failed to update menu", err)
		return
	}

	response.SuccessWithMessage(c, "menu updated successfully", menu)
}

// DeleteMenu deletes a menu.
func (a *MenuManagementAPI) DeleteMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	if err := a.menuService.DeleteMenuContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemMenuServiceError(c, "failed to delete menu", err)
		return
	}

	response.SuccessWithMessage(c, "menu deleted successfully", nil)
}
