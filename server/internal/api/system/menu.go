package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// MenuManagementAPI 菜单管理API
type MenuManagementAPI struct {
	menuService system.MenuService
}

// NewMenuManagementAPI 创建MenuManagementAPI实例
func NewMenuManagementAPI() *MenuManagementAPI {
	return &MenuManagementAPI{
		menuService: system.MenuService{},
	}
}

// GetMenuList 获取菜单列表
func (a *MenuManagementAPI) GetMenuList(c *gin.Context) {
	var req system.MenuListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	// 解析状态参数
	if statusStr := c.Query("status"); statusStr != "" {
		status, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(status)
			req.Status = &statusInt8
		}
	}

	menus, total, err := a.menuService.GetMenuList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, menus, total, req.Page, req.PageSize)
}

// GetMenuTree 获取菜单树
func (a *MenuManagementAPI) GetMenuTree(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	menus, err := a.menuService.GetMenuTree(status)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, menus)
}

// GetMenu 获取菜单详情
func (a *MenuManagementAPI) GetMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	menu, err := a.menuService.GetMenuByID(uint(id))
	if err != nil {
		response.NotFound(c, "menu not found")
		return
	}

	response.Success(c, menu)
}

// CreateMenu 创建菜单
func (a *MenuManagementAPI) CreateMenu(c *gin.Context) {
	var req system.CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	menu, err := a.menuService.CreateMenu(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "menu created successfully", menu)
}

// UpdateMenu 更新菜单
func (a *MenuManagementAPI) UpdateMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	var req system.UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	menu, err := a.menuService.UpdateMenu(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "menu updated successfully", menu)
}

// DeleteMenu 删除菜单
func (a *MenuManagementAPI) DeleteMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid menu id")
		return
	}

	if err := a.menuService.DeleteMenu(uint(id)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "menu deleted successfully", nil)
}
