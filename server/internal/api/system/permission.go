package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// PermissionManagementAPI 权限管理API
type PermissionManagementAPI struct {
	permissionService system.PermissionService
}

// NewPermissionManagementAPI 创建PermissionManagementAPI实例
func NewPermissionManagementAPI() *PermissionManagementAPI {
	return &PermissionManagementAPI{
		permissionService: system.PermissionService{},
	}
}

// GetPermissionList 获取权限列表
func (a *PermissionManagementAPI) GetPermissionList(c *gin.Context) {
	var req system.PermissionListRequest
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

	// 解析类型参数
	if typeStr := c.Query("type"); typeStr != "" {
		permissionType, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			typeInt8 := int8(permissionType)
			req.Type = &typeInt8
		}
	}

	permissions, total, err := a.permissionService.GetPermissionList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, permissions, total, req.Page, req.PageSize)
}

// GetPermissionTree 获取权限树
func (a *PermissionManagementAPI) GetPermissionTree(c *gin.Context) {
	permissions, err := a.permissionService.GetPermissionTree()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, permissions)
}

// GetPermission 获取权限详情
func (a *PermissionManagementAPI) GetPermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	permission, err := a.permissionService.GetPermissionByID(uint(id))
	if err != nil {
		response.NotFound(c, "permission not found")
		return
	}

	response.Success(c, permission)
}

// CreatePermission 创建权限
func (a *PermissionManagementAPI) CreatePermission(c *gin.Context) {
	var req system.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	permission, err := a.permissionService.CreatePermission(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "permission created successfully", permission)
}

// UpdatePermission 更新权限
func (a *PermissionManagementAPI) UpdatePermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	var req system.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	permission, err := a.permissionService.UpdatePermission(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "permission updated successfully", permission)
}

// DeletePermission 删除权限
func (a *PermissionManagementAPI) DeletePermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	if err := a.permissionService.DeletePermission(uint(id)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "permission deleted successfully", nil)
}
