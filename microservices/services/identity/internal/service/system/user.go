package system

import (
	"context"
	"errors"
	"time"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	authsvc "github.com/go-admin-kit/services/identity/internal/service/auth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService manages users for the system module.
type UserService struct {
	userDAO   systemdao.UserDAO
	tenantDAO *systemdao.TenantDAO
}

// NewUserServiceWithDB builds a UserService backed by an injected database handle.
func NewUserServiceWithDB(db *gorm.DB) UserService {
	return UserService{
		userDAO:   *systemdao.NewUserDAO(db),
		tenantDAO: systemdao.NewTenantDAO(db),
	}
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
	ErrRoleNotInTenant       = errors.New("role does not belong to current tenant")
	ErrDepartmentNotInTenant = errors.New("department does not belong to current tenant")
	ErrTenantUserQuota       = errors.New("tenant user quota exceeded")
)

func (s *UserService) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if err := assertSameTenant(ctx, user.TenantID); err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	user, err := s.userDAO.GetUserWithRolesContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if err := assertSameTenant(ctx, user.TenantID); err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *UserService) GetUserListContext(ctx context.Context, req UserListRequest) ([]model.User, int64, error) {
	return s.userDAO.GetUserListContext(ctx, req.PageRequest, req.Keyword, req.Status, req.DataScope)
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

	if err := authsvc.ValidatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("password hashing failed")
	}

	tenantID := tenant.Normalize(tenant.FromContext(ctx))
	if err := s.enforceUserQuota(ctx, tenantID); err != nil {
		return nil, err
	}
	if req.DepartmentID > 0 {
		if err := s.userDAO.AssertDepartmentInTenantContext(ctx, req.DepartmentID, tenantID); err != nil {
			if errors.Is(err, systemdao.ErrDepartmentNotInTenant) {
				return nil, ErrDepartmentNotInTenant
			}
			return nil, err
		}
	}
	now := time.Now()
	user := &model.User{
		TenantID:          tenantID,
		Username:          req.Username,
		Password:          string(hashedPassword),
		Nickname:          req.Nickname,
		Email:             req.Email,
		Phone:             req.Phone,
		DepartmentID:      req.DepartmentID,
		Status:            req.Status,
		PasswordChangedAt: &now,
	}

	if user.Status == 0 {
		user.Status = 1
	}

	if err := s.userDAO.CreateUserContext(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) UpdateUserContext(ctx context.Context, id uint, req UpdateUserRequest) (*model.User, error) {
	user, err := s.GetUserByIDContext(ctx, id)
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

func (s *UserService) DeleteUserContext(ctx context.Context, id uint) error {
	if _, err := s.GetUserByIDContext(ctx, id); err != nil {
		return err
	}
	return s.userDAO.DeleteUserContext(ctx, id)
}

func (s *UserService) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	if _, err := s.GetUserByIDContext(ctx, id); err != nil {
		return err
	}
	return s.userDAO.UpdateUserStatusContext(ctx, id, status)
}

func (s *UserService) AssignRolesContext(ctx context.Context, userID uint, req AssignRolesRequest) error {
	user, err := s.GetUserByIDContext(ctx, userID)
	if err != nil {
		if isContextError(err) {
			return err
		}
		return ErrUserNotFound
	}

	if err := s.userDAO.AssertRolesInTenantContext(ctx, req.RoleIDs, user.TenantID); err != nil {
		if errors.Is(err, systemdao.ErrRoleNotInTenant) {
			return ErrRoleNotInTenant
		}
		return err
	}

	if err := s.userDAO.AssignRolesContext(ctx, userID, req.RoleIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsersContext(ctx, userID)
}

func assertSameTenant(ctx context.Context, resourceTenantID uint) error {
	actor := tenant.FromContext(ctx)
	if actor == 0 {
		return nil // no tenant in context (internal jobs); allow
	}
	if resourceTenantID == 0 {
		resourceTenantID = 1
	}
	if actor != resourceTenantID {
		return ErrUserNotFound
	}
	return nil
}

func (s *UserService) enforceUserQuota(ctx context.Context, tenantID uint) error {
	if s.tenantDAO == nil {
		return nil
	}
	// Platform-wide quota read bypasses actor tenant row filter.
	qctx := tenant.DisableScope(ctx)
	t, err := s.tenantDAO.GetByIDContext(qctx, tenantID)
	if err != nil {
		// No tenants table / missing row → skip enforcement (single-tenant legacy)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if t.MaxUsers <= 0 {
		return nil
	}
	n, err := s.tenantDAO.CountUsersContext(qctx, tenantID)
	if err != nil {
		return err
	}
	if n >= t.MaxUsers {
		return ErrTenantUserQuota
	}
	return nil
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
