package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// RoleManagementAPI 角色管理API
type RoleManagementAPI struct {
	roleService system.RoleService
}

// NewRoleManagementAPI 创建RoleManagementAPI实例
func NewRoleManagementAPI() *RoleManagementAPI {
	return &RoleManagementAPI{
		roleService: system.RoleService{},
	}
}

// GetRoleList 获取角色列表
func (a *RoleManagementAPI) GetRoleList(c *gin.Context) {
	var req system.RoleListRequest
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

	roles, total, err := a.roleService.GetRoleList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, roles, total, req.Page, req.PageSize)
}

// GetAllRoles 获取所有角色
func (a *RoleManagementAPI) GetAllRoles(c *gin.Context) {
	roles, err := a.roleService.GetAllRoles()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, roles)
}

// GetRole 获取角色详情
func (a *RoleManagementAPI) GetRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	role, err := a.roleService.GetRoleByID(uint(id))
	if err != nil {
		response.NotFound(c, "role not found")
		return
	}

	response.Success(c, role)
}

// CreateRole 创建角色
func (a *RoleManagementAPI) CreateRole(c *gin.Context) {
	var req system.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	role, err := a.roleService.CreateRole(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "role created successfully", role)
}

// UpdateRole 更新角色
func (a *RoleManagementAPI) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var req system.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	role, err := a.roleService.UpdateRole(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "role updated successfully", role)
}

// DeleteRole 删除角色
func (a *RoleManagementAPI) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	if err := a.roleService.DeleteRole(uint(id)); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "role deleted successfully", nil)
}

// AssignPermissions 分配权限
func (a *RoleManagementAPI) AssignPermissions(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var req system.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.roleService.AssignPermissions(uint(id), req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "permissions assigned successfully", nil)
}
