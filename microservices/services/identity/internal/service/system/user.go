package system

import (
	"context"
	"errors"
	"time"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	authsvc "github.com/go-admin-kit/services/identity/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// userStatusEnabled is the users.status value for an active account; anything
// else denies login (see AuthMiddleware's cookie-path check).
const userStatusEnabled int8 = 1

// UserService manages users for the system module.
type UserService struct {
	userDAO   systemdao.UserDAO
	tenantDAO *systemdao.TenantDAO
	// deptDAO 导出（部门名列）与导入（部门名→id 解析）用；零值构造时为
	// nil，相关方法自行降级（见 user_excel.go）。
	deptDAO *systemdao.DepartmentDAO
}

// NewUserServiceWithDB builds a UserService backed by an injected database handle.
func NewUserServiceWithDB(db *gorm.DB) UserService {
	return UserService{
		userDAO:   *systemdao.NewUserDAO(db),
		tenantDAO: systemdao.NewTenantDAO(db),
		deptDAO:   systemdao.NewDepartmentDAO(db),
	}
}

type UserListRequest struct {
	pagination.PageRequest
	Keyword   string              `json:"keyword" form:"keyword"`
	Status    *int8               `json:"status" form:"status"`
	DataScope authz.UserDataScope `json:"-" form:"-"`
}

type UpdateUserRequest struct {
	Nickname string  `json:"nickname"`
	Email    string  `json:"email" binding:"email"`
	Phone    string  `json:"phone"`
	Avatar   string  `json:"avatar"`
	PostIDs  *[]uint `json:"post_ids"`
	// 指针语义：nil = 不改；0 = 移出部门；>0 = 换部门（校验租户归属）
	DepartmentID *uint `json:"department_id"`
}

type ResetPasswordRequest struct {
	// min=8 aligns with auth's ValidatePasswordStrength — the binding tag is a
	// fast-fail; the service still enforces the full strength policy.
	Password string `json:"password" binding:"required,min=8"`
	// MustChange defaults to true when omitted: an admin-set password is a
	// temporary credential, so forcing a change is the safe default.
	MustChange *bool `json:"must_change"`
}

type AssignRolesRequest struct {
	RoleIDs []uint `json:"role_ids" binding:"required"`
}

type CreateUserRequest struct {
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required,min=8"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	DepartmentID uint   `json:"department_id"`
	Status       int8   `json:"status"`
	PostIDs      []uint `json:"post_ids"`
	// MustChange 管理员代建账号的临时密码默认强制改密；显式 false 可关闭。
	MustChange *bool `json:"must_change"`
}

var (
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrUserNotFound          = errors.New("user not found")
	ErrRoleNotInTenant       = errors.New("role does not belong to current tenant")
	ErrDepartmentNotInTenant = errors.New("department does not belong to current tenant")
	ErrPostNotInTenant       = errors.New("post does not belong to current tenant")
	ErrTenantUserQuota       = errors.New("tenant user quota exceeded")
)

// fillPostIDs mirrors preloaded posts into the plain post_ids field.
func fillPostIDs(user *localmodel.User) {
	if user == nil || len(user.Posts) == 0 {
		return
	}
	ids := make([]uint, 0, len(user.Posts))
	for _, post := range user.Posts {
		ids = append(ids, post.ID)
	}
	user.PostIDs = ids
}

func (s *UserService) GetUserByIDContext(ctx context.Context, id uint) (*localmodel.User, error) {
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

func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*localmodel.User, error) {
	user, err := s.userDAO.GetUserWithRolesPostsContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if err := assertSameTenant(ctx, user.TenantID); err != nil {
		return nil, ErrUserNotFound
	}
	fillPostIDs(user)
	return user, nil
}

func (s *UserService) GetUserListContext(ctx context.Context, req UserListRequest) ([]localmodel.User, int64, error) {
	users, total, err := s.userDAO.GetUserListContext(ctx, req.PageRequest, req.Keyword, req.Status, req.DataScope)
	if err != nil {
		return nil, 0, err
	}
	for i := range users {
		fillPostIDs(&users[i])
	}
	return users, total, nil
}

func (s *UserService) CreateUserContext(ctx context.Context, req CreateUserRequest) (*localmodel.User, error) {
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
	if len(req.PostIDs) > 0 {
		if err := s.userDAO.AssertPostsInTenantContext(ctx, req.PostIDs, tenantID); err != nil {
			if errors.Is(err, systemdao.ErrPostNotInTenant) {
				return nil, ErrPostNotInTenant
			}
			return nil, err
		}
	}
	now := time.Now()
	// 管理员代建默认强制首次改密（临时凭据）；显式 MustChange=false 可关。
	mustChange := req.MustChange == nil || *req.MustChange
	user := &localmodel.User{
		TenantID:           tenantID,
		Username:           req.Username,
		Password:           string(hashedPassword),
		Nickname:           req.Nickname,
		Email:              req.Email,
		Phone:              req.Phone,
		DepartmentID:       req.DepartmentID,
		Status:             req.Status,
		MustChangePassword: mustChange,
		PasswordChangedAt:  &now,
	}

	if user.Status == 0 {
		user.Status = 1
	}

	if err := s.userDAO.CreateUserContext(ctx, user); err != nil {
		return nil, err
	}

	if len(req.PostIDs) > 0 {
		if err := s.userDAO.AssignPostsContext(ctx, user.ID, req.PostIDs); err != nil {
			return nil, err
		}
		user.PostIDs = req.PostIDs
	}

	return user, nil
}

func (s *UserService) UpdateUserContext(ctx context.Context, id uint, req UpdateUserRequest) (*localmodel.User, error) {
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
	if req.DepartmentID != nil {
		if *req.DepartmentID > 0 {
			if err := s.userDAO.AssertDepartmentInTenantContext(ctx, *req.DepartmentID, user.TenantID); err != nil {
				if errors.Is(err, systemdao.ErrDepartmentNotInTenant) {
					return nil, ErrDepartmentNotInTenant
				}
				return nil, err
			}
		}
		user.DepartmentID = *req.DepartmentID
	}

	if err := s.userDAO.UpdateUserContext(ctx, user); err != nil {
		return nil, err
	}

	if req.PostIDs != nil {
		if err := s.userDAO.AssertPostsInTenantContext(ctx, *req.PostIDs, user.TenantID); err != nil {
			if errors.Is(err, systemdao.ErrPostNotInTenant) {
				return nil, ErrPostNotInTenant
			}
			return nil, err
		}
		if err := s.userDAO.AssignPostsContext(ctx, user.ID, *req.PostIDs); err != nil {
			return nil, err
		}
		user.PostIDs = *req.PostIDs
	}

	return user, nil
}

func (s *UserService) DeleteUserContext(ctx context.Context, id uint) error {
	if _, err := s.GetUserByIDContext(ctx, id); err != nil {
		return err
	}
	if err := s.userDAO.DeleteUserContext(ctx, id); err != nil {
		return err
	}
	return revokeUserAccessContext(ctx, id)
}

func (s *UserService) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	if _, err := s.GetUserByIDContext(ctx, id); err != nil {
		return err
	}
	if err := s.userDAO.UpdateUserStatusContext(ctx, id, status); err != nil {
		return err
	}
	if status == userStatusEnabled {
		return InvalidatePermissionCacheForUsersContext(ctx, id)
	}
	return revokeUserAccessContext(ctx, id)
}

// ResetUserPasswordContext 管理员重置用户密码：校验强度、改密后吊销该用户
// 全部会话，并置 must_change_password 强制其下次登录改密。
func (s *UserService) ResetUserPasswordContext(ctx context.Context, id uint, req ResetPasswordRequest) error {
	user, err := s.GetUserByIDContext(ctx, id)
	if err != nil {
		return err
	}
	if err := authsvc.ValidatePasswordStrength(req.Password); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("password hashing failed")
	}
	now := time.Now()
	user.Password = string(hashed)
	user.PasswordChangedAt = &now
	user.MustChangePassword = req.MustChange == nil || *req.MustChange
	if err := s.userDAO.UpdateUserContext(ctx, user); err != nil {
		return err
	}
	return revokeUserAccessContext(ctx, user.ID)
}

// revokeUserAccessContext cuts off an account that was just disabled or deleted.
// Dropping the permission cache alone is not enough: AuthMiddleware only re-reads
// users.status on the cookie path, so an already-issued Bearer token would keep
// working until it expires. Blacklisting the live tokens is what actually ends
// the session; the cache drop stops a stale permission set from being served to
// any token issued before the change.
func revokeUserAccessContext(ctx context.Context, userID uint) error {
	if err := InvalidatePermissionCacheForUsersContext(ctx, userID); err != nil {
		return err
	}
	return NewOnlineUserService().RevokeUserTokensContext(ctx, userID)
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
