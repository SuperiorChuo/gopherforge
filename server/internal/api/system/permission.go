package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// PermissionManagementAPI handles permission management endpoints.
type PermissionManagementAPI struct {
	permissionService system.PermissionService
}

// NewPermissionManagementAPI creates a PermissionManagementAPI instance.
func NewPermissionManagementAPI() *PermissionManagementAPI {
	return &PermissionManagementAPI{
		permissionService: system.PermissionService{},
	}
}

// GetPermissionList returns paginated permissions.
func (a *PermissionManagementAPI) GetPermissionList(c *gin.Context) {
	var req system.PermissionListRequest
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

	// Parse type from query separately because Gin does not bind *int8 reliably.
	if typeStr := c.Query("type"); typeStr != "" {
		permissionType, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			typeInt8 := int8(permissionType)
			req.Type = &typeInt8
		}
	}

	permissions, total, err := a.permissionService.GetPermissionListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get permission list", err)
		return
	}

	response.PageSuccess(c, permissions, total, req.Page, req.PageSize)
}

// GetPermissionTree returns permissions as a tree.
func (a *PermissionManagementAPI) GetPermissionTree(c *gin.Context) {
	permissions, err := a.permissionService.GetPermissionTreeContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get permission tree", err)
		return
	}

	response.Success(c, permissions)
}

// GetPermission returns a permission by id.
func (a *PermissionManagementAPI) GetPermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	permission, err := a.permissionService.GetPermissionByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemPermissionServiceError(c, "failed to get permission", err)
		return
	}

	response.Success(c, permission)
}

// CreatePermission creates a permission.
func (a *PermissionManagementAPI) CreatePermission(c *gin.Context) {
	var req system.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	permission, err := a.permissionService.CreatePermissionContext(c.Request.Context(), req)
	if err != nil {
		writeSystemPermissionServiceError(c, "failed to create permission", err)
		return
	}

	response.SuccessWithMessage(c, "permission created successfully", permission)
}

// UpdatePermission updates a permission.
func (a *PermissionManagementAPI) UpdatePermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	var req system.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	permission, err := a.permissionService.UpdatePermissionContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemPermissionServiceError(c, "failed to update permission", err)
		return
	}

	response.SuccessWithMessage(c, "permission updated successfully", permission)
}

// DeletePermission deletes a permission.
func (a *PermissionManagementAPI) DeletePermission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid permission id")
		return
	}

	if err := a.permissionService.DeletePermissionContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemPermissionServiceError(c, "failed to delete permission", err)
		return
	}

	response.SuccessWithMessage(c, "permission deleted successfully", nil)
}
