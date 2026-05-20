package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"golang.org/x/crypto/bcrypt"
)

// UserService 用户管理服务
type UserService struct {
	userDAO system.UserDAO
}

// UserListRequest 用户列表请求
type UserListRequest struct {
	pagination.PageRequest
	Keyword   string              `json:"keyword" form:"keyword"`
	Status    *int8               `json:"status" form:"status"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email" binding:"email"`
	Phone    string `json:"phone"`
	Avatar   string `json:"avatar"`
}

// AssignRolesRequest 分配角色请求
type AssignRolesRequest struct {
	RoleIDs []uint `json:"role_ids" binding:"required"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required,min=6"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	DepartmentID uint   `json:"department_id"`
	Status       int8   `json:"status"`
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(id uint) (*model.User, error) {
	return s.userDAO.GetUserByID(id)
}

// GetUserWithRoles 获取用户及其角色
func (s *UserService) GetUserWithRoles(id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRoles(id)
}

// GetUserList 获取用户列表
func (s *UserService) GetUserList(req UserListRequest) ([]model.User, int64, error) {
	return s.userDAO.GetUserList(req.PageRequest, req.Keyword, req.Status, req.DataScope)
}

// CreateUser 创建用户
func (s *UserService) CreateUser(req CreateUserRequest) (*model.User, error) {
	// 检查用户名是否已存在
	_, err := s.userDAO.GetUserByUsername(req.Username)
	if err == nil {
		return nil, errors.New("用户名已存在")
	}

	// 检查邮箱是否已存在
	if req.Email != "" {
		_, err := s.userDAO.GetUserByEmail(req.Email)
		if err == nil {
			return nil, errors.New("邮箱已存在")
		}
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	// 创建用户
	user := &model.User{
		Username:     req.Username,
		Password:     string(hashedPassword),
		Nickname:     req.Nickname,
		Email:        req.Email,
		Phone:        req.Phone,
		DepartmentID: req.DepartmentID,
		Status:       req.Status,
	}

	// 默认启用状态
	if user.Status == 0 {
		user.Status = 1
	}

	if err := s.userDAO.CreateUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUser 更新用户信息
func (s *UserService) UpdateUser(id uint, req UpdateUserRequest) (*model.User, error) {
	user, err := s.userDAO.GetUserByID(id)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// 如果更新邮箱，检查是否已存在
	if req.Email != "" && req.Email != user.Email {
		_, err := s.userDAO.GetUserByEmail(req.Email)
		if err == nil {
			return nil, errors.New("email already exists")
		}
		user.Email = req.Email
	}

	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}

	if err := s.userDAO.UpdateUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(id uint) error {
	return s.userDAO.DeleteUser(id)
}

// UpdateUserStatus 更新用户状态
func (s *UserService) UpdateUserStatus(id uint, status int8) error {
	return s.userDAO.UpdateUserStatus(id, status)
}

// AssignRoles 分配角色
func (s *UserService) AssignRoles(userID uint, req AssignRolesRequest) error {
	// 检查用户是否存在
	_, err := s.userDAO.GetUserByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	if err := s.userDAO.AssignRoles(userID, req.RoleIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsers(userID)
}
