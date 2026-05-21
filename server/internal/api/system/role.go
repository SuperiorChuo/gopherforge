package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// RoleManagementAPI handles role management endpoints.
type RoleManagementAPI struct {
	roleService system.RoleService
}

// NewRoleManagementAPI creates a RoleManagementAPI instance.
func NewRoleManagementAPI() *RoleManagementAPI {
	return &RoleManagementAPI{
		roleService: system.RoleService{},
	}
}

// GetRoleList returns paginated roles.
func (a *RoleManagementAPI) GetRoleList(c *gin.Context) {
	var req system.RoleListRequest
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

	roles, total, err := a.roleService.GetRoleListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get role list", err)
		return
	}

	response.PageSuccess(c, roles, total, req.Page, req.PageSize)
}

// GetAllRoles returns all roles.
func (a *RoleManagementAPI) GetAllRoles(c *gin.Context) {
	roles, err := a.roleService.GetAllRolesContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get roles", err)
		return
	}

	response.Success(c, roles)
}

// GetRole returns a role by id.
func (a *RoleManagementAPI) GetRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	role, err := a.roleService.GetRoleByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemRoleServiceError(c, "failed to get role", err)
		return
	}

	response.Success(c, role)
}

// CreateRole creates a role.
func (a *RoleManagementAPI) CreateRole(c *gin.Context) {
	var req system.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	role, err := a.roleService.CreateRoleContext(c.Request.Context(), req)
	if err != nil {
		writeSystemRoleServiceError(c, "failed to create role", err)
		return
	}

	response.SuccessWithMessage(c, "role created successfully", role)
}

// UpdateRole updates a role.
func (a *RoleManagementAPI) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var req system.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	role, err := a.roleService.UpdateRoleContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemRoleServiceError(c, "failed to update role", err)
		return
	}

	response.SuccessWithMessage(c, "role updated successfully", role)
}

// DeleteRole deletes a role.
func (a *RoleManagementAPI) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	if err := a.roleService.DeleteRoleContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemRoleServiceError(c, "failed to delete role", err)
		return
	}

	response.SuccessWithMessage(c, "role deleted successfully", nil)
}

// AssignPermissions assigns permissions to a role.
func (a *RoleManagementAPI) AssignPermissions(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var req system.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := a.roleService.AssignPermissionsContext(c.Request.Context(), uint(id), req); err != nil {
		writeSystemRoleServiceError(c, "failed to assign role permissions", err)
		return
	}

	response.SuccessWithMessage(c, "permissions assigned successfully", nil)
}
