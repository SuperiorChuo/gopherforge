package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// UserManagementAPI handles user management endpoints.
type UserManagementAPI struct {
	userService system.UserService
}

// NewUserManagementAPI creates a UserManagementAPI instance.
func NewUserManagementAPI() *UserManagementAPI {
	return &UserManagementAPI{
		userService: system.UserService{},
	}
}

// CreateUser creates a user.
func (a *UserManagementAPI) CreateUser(c *gin.Context) {
	var req system.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	user, err := a.userService.CreateUserContext(c.Request.Context(), req)
	if err != nil {
		writeSystemUserServiceError(c, "failed to create user", err)
		return
	}

	response.SuccessWithMessage(c, "user created successfully", user)
}

// GetUserList returns paginated users.
func (a *UserManagementAPI) GetUserList(c *gin.Context) {
	var req system.UserListRequest
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

	// Parse status from query separately because Gin does not bind *int8 reliably.
	if statusStr := c.Query("status"); statusStr != "" {
		status, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(status)
			req.Status = &statusInt8
		}
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve user data scope", err)
		return
	}
	req.DataScope = dataScope

	users, total, err := a.userService.GetUserListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get user list", err)
		return
	}

	// Keep the response shape expected by the frontend table.
	response.Success(c, gin.H{
		"list":      users,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetUser returns a user by id.
func (a *UserManagementAPI) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	user, err := a.userService.GetUserWithRolesContext(c.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, user)
}

// UpdateUser updates a user.
func (a *UserManagementAPI) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	user, err := a.userService.UpdateUserContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemUserServiceError(c, "failed to update user", err)
		return
	}

	response.Success(c, user)
}

// DeleteUser deletes a user.
func (a *UserManagementAPI) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	if err := a.userService.DeleteUserContext(c.Request.Context(), uint(id)); err != nil {
		internalServerError(c, "failed to delete user", err)
		return
	}

	response.SuccessWithMessage(c, "user deleted successfully", nil)
}

// UpdateUserStatus updates a user's status.
func (a *UserManagementAPI) UpdateUserStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req struct {
		Status int8 `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := a.userService.UpdateUserStatusContext(c.Request.Context(), uint(id), req.Status); err != nil {
		internalServerError(c, "failed to update user status", err)
		return
	}

	response.SuccessWithMessage(c, "user status updated successfully", nil)
}

// AssignRoles assigns roles to a user.
func (a *UserManagementAPI) AssignRoles(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := a.userService.AssignRolesContext(c.Request.Context(), uint(id), req); err != nil {
		writeSystemUserServiceError(c, "failed to assign user roles", err)
		return
	}

	response.SuccessWithMessage(c, "roles assigned successfully", nil)
}
