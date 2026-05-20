package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// UserManagementAPI 用户管理API
type UserManagementAPI struct {
	userService system.UserService
}

// NewUserManagementAPI 创建UserManagementAPI实例
func NewUserManagementAPI() *UserManagementAPI {
	return &UserManagementAPI{
		userService: system.UserService{},
	}
}

// CreateUser 创建用户
func (a *UserManagementAPI) CreateUser(c *gin.Context) {
	var req system.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := a.userService.CreateUser(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "用户创建成功", user)
}

// GetUserList 获取用户列表
func (a *UserManagementAPI) GetUserList(c *gin.Context) {
	var req system.UserListRequest
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

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	req.DataScope = dataScope

	users, total, err := a.userService.GetUserList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	// 前端期望字段：list、total、page、page_size
	response.Success(c, gin.H{
		"list":      users,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetUser 获取用户详情
func (a *UserManagementAPI) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	user, err := a.userService.GetUserWithRoles(uint(id))
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, user)
}

// UpdateUser 更新用户信息
func (a *UserManagementAPI) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := a.userService.UpdateUser(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, user)
}

// DeleteUser 删除用户
func (a *UserManagementAPI) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	if err := a.userService.DeleteUser(uint(id)); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "user deleted successfully", nil)
}

// UpdateUserStatus 更新用户状态
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
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.userService.UpdateUserStatus(uint(id), req.Status); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "user status updated successfully", nil)
}

// AssignRoles 分配角色
func (a *UserManagementAPI) AssignRoles(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req system.AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.userService.AssignRoles(uint(id), req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "roles assigned successfully", nil)
}
