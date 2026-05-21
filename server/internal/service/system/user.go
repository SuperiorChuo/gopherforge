package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService manages users for the system module.
type UserService struct {
	userDAO systemdao.UserDAO
}

type UserListRequest struct {
	pagination.PageRequest
	Keyword   string              `json:"keyword" form:"keyword"`
	Status    *int8               `json:"status" form:"status"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

type UpdateUserRequest struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email" binding:"email"`
	Phone    string `json:"phone"`
	Avatar   string `json:"avatar"`
}

type AssignRolesRequest struct {
	RoleIDs []uint `json:"role_ids" binding:"required"`
}

type CreateUserRequest struct {
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required,min=6"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	DepartmentID uint   `json:"department_id"`
	Status       int8   `json:"status"`
}

var (
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrUserNotFound          = errors.New("user not found")
)

// Deprecated: use GetUserByIDContext instead.
func (s *UserService) GetUserByID(id uint) (*model.User, error) {
	return s.GetUserByIDContext(context.Background(), id)
}

func (s *UserService) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserByIDContext(ctx, id)
}

// Deprecated: use GetUserWithRolesContext instead.
func (s *UserService) GetUserWithRoles(id uint) (*model.User, error) {
	return s.GetUserWithRolesContext(context.Background(), id)
}

func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesContext(ctx, id)
}

// Deprecated: use GetUserListContext instead.
func (s *UserService) GetUserList(req UserListRequest) ([]model.User, int64, error) {
	return s.GetUserListContext(context.Background(), req)
}

func (s *UserService) GetUserListContext(ctx context.Context, req UserListRequest) ([]model.User, int64, error) {
	return s.userDAO.GetUserListContext(ctx, req.PageRequest, req.Keyword, req.Status, req.DataScope)
}

// Deprecated: use CreateUserContext instead.
func (s *UserService) CreateUser(req CreateUserRequest) (*model.User, error) {
	return s.CreateUserContext(context.Background(), req)
}

func (s *UserService) CreateUserContext(ctx context.Context, req CreateUserRequest) (*model.User, error) {
	_, err := s.userDAO.GetUserByUsernameContext(ctx, req.Username)
	if err == nil {
		return nil, ErrUsernameAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if req.Email != "" {
		_, err := s.userDAO.GetUserByEmailContext(ctx, req.Email)
		if err == nil {
			return nil, ErrEmailAlreadyExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("password hashing failed")
	}

	user := &model.User{
		Username:     req.Username,
		Password:     string(hashedPassword),
		Nickname:     req.Nickname,
		Email:        req.Email,
		Phone:        req.Phone,
		DepartmentID: req.DepartmentID,
		Status:       req.Status,
	}

	if user.Status == 0 {
		user.Status = 1
	}

	if err := s.userDAO.CreateUserContext(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Deprecated: use UpdateUserContext instead.
func (s *UserService) UpdateUser(id uint, req UpdateUserRequest) (*model.User, error) {
	return s.UpdateUserContext(context.Background(), id, req)
}

func (s *UserService) UpdateUserContext(ctx context.Context, id uint, req UpdateUserRequest) (*model.User, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, id)
	if err != nil {
		if isContextError(err) {
			return nil, err
		}
		return nil, ErrUserNotFound
	}

	if req.Email != "" && req.Email != user.Email {
		_, err := s.userDAO.GetUserByEmailContext(ctx, req.Email)
		if err == nil {
			return nil, ErrEmailAlreadyExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
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

	if err := s.userDAO.UpdateUserContext(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Deprecated: use DeleteUserContext instead.
func (s *UserService) DeleteUser(id uint) error {
	return s.DeleteUserContext(context.Background(), id)
}

func (s *UserService) DeleteUserContext(ctx context.Context, id uint) error {
	return s.userDAO.DeleteUserContext(ctx, id)
}

// Deprecated: use UpdateUserStatusContext instead.
func (s *UserService) UpdateUserStatus(id uint, status int8) error {
	return s.UpdateUserStatusContext(context.Background(), id, status)
}

func (s *UserService) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	return s.userDAO.UpdateUserStatusContext(ctx, id, status)
}

// Deprecated: use AssignRolesContext instead.
func (s *UserService) AssignRoles(userID uint, req AssignRolesRequest) error {
	return s.AssignRolesContext(context.Background(), userID, req)
}

func (s *UserService) AssignRolesContext(ctx context.Context, userID uint, req AssignRolesRequest) error {
	_, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if isContextError(err) {
			return err
		}
		return ErrUserNotFound
	}

	if err := s.userDAO.AssignRolesContext(ctx, userID, req.RoleIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsersContext(ctx, userID)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
