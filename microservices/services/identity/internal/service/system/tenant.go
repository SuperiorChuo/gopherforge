package system

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrTenantNotFound      = errors.New("tenant not found")
	ErrTenantCodeExists    = errors.New("tenant code already exists")
	ErrTenantCodeInvalid   = errors.New("tenant code must be lowercase letters, digits, hyphen (2-64)")
	ErrTenantNameRequired  = errors.New("tenant name required")
	ErrDefaultTenantLocked = errors.New("default tenant code cannot be changed")
)

var tenantCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// TenantService manages SaaS tenants.
type TenantService struct {
	dao    *system.TenantDAO
	pkgDAO *system.TenantPackageDAO
	// db 供开通 provisioning / 删除级联的子服务与事务使用。
	db *gorm.DB
}

func NewTenantServiceWithDB(db *gorm.DB) *TenantService {
	return &TenantService{
		dao:    system.NewTenantDAO(db),
		pkgDAO: system.NewTenantPackageDAO(db),
		db:     db,
	}
}

// ProvisionedAdmin 是开通租户时自动创建的初始管理员凭据（一次性返回给平台管理员转交）。
type ProvisionedAdmin struct {
	Username        string `json:"username"`
	InitialPassword string `json:"initial_password"`
}

// resolvePackageID 归一化套餐绑定入参：nil/0 → 不绑定（NULL）；>0 校验套餐存在后绑定。
func (s *TenantService) resolvePackageID(ctx context.Context, packageID *uint) (*uint, error) {
	if packageID == nil || *packageID == 0 {
		return nil, nil
	}
	if s.pkgDAO == nil {
		return nil, ErrTenantPackageNotFound
	}
	if _, err := s.pkgDAO.GetByIDContext(ctx, *packageID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantPackageNotFound
		}
		return nil, err
	}
	id := *packageID
	return &id, nil
}

type TenantListRequest struct {
	pagination.PageRequest
	Keyword string
	Status  *int8
}

type CreateTenantRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Plan     string `json:"plan"`
	MaxUsers int64  `json:"max_users"`
	Status   int8   `json:"status"`
	// PackageID 租户套餐（权限包）；缺省/0 = 不限。
	PackageID *uint `json:"package_id"`
}

// PlanDefaultMaxUsers returns a soft default quota when max_users is 0 on create.
func PlanDefaultMaxUsers(plan string) int64 {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "pro":
		return 50
	case "enterprise":
		return 0 // unlimited
	default: // free
		return 10
	}
}

type UpdateTenantRequest struct {
	Name     *string `json:"name"`
	Plan     *string `json:"plan"`
	MaxUsers *int64  `json:"max_users"`
	Status   *int8   `json:"status"`
	// PackageID 租户套餐：缺省 = 不改；0 = 解绑（置 NULL）；>0 = 绑定（校验存在）。
	PackageID *uint `json:"package_id"`
}

func (s *TenantService) List(ctx context.Context, req TenantListRequest) ([]model.Tenant, int64, error) {
	// Platform-wide catalog: disable row tenant plugin (tenants table has no tenant_id).
	ctx = tenant.DisableScope(ctx)
	return s.dao.ListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *TenantService) Get(ctx context.Context, id uint) (*model.Tenant, error) {
	ctx = tenant.DisableScope(ctx)
	t, err := s.dao.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *TenantService) Create(ctx context.Context, req CreateTenantRequest) (*model.Tenant, *ProvisionedAdmin, error) {
	ctx = tenant.DisableScope(ctx)
	code := strings.ToLower(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	if !tenantCodeRe.MatchString(code) {
		return nil, nil, ErrTenantCodeInvalid
	}
	if name == "" {
		return nil, nil, ErrTenantNameRequired
	}
	if _, err := s.dao.GetByCodeContext(ctx, code); err == nil {
		return nil, nil, ErrTenantCodeExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	plan := strings.TrimSpace(req.Plan)
	if plan == "" {
		plan = "free"
	}
	maxUsers := req.MaxUsers
	if maxUsers == 0 && plan != "enterprise" {
		// 0 means "use plan default" on create (enterprise stays unlimited).
		maxUsers = PlanDefaultMaxUsers(plan)
	}
	packageID, err := s.resolvePackageID(ctx, req.PackageID)
	if err != nil {
		return nil, nil, err
	}
	t := &model.Tenant{
		Code:      code,
		Name:      name,
		Status:    status,
		Plan:      plan,
		MaxUsers:  maxUsers,
		PackageID: packageID,
	}
	if err := s.dao.CreateContext(ctx, t); err != nil {
		return nil, nil, err
	}
	// 开通：建初始管理员角色 + 授权 + 管理员用户。失败则回滚租户行，保持原子。
	admin, err := s.provision(t)
	if err != nil {
		_ = s.dao.DeleteContext(tenant.WithContext(context.Background(), t.ID), t.ID)
		return nil, nil, fmt.Errorf("租户已创建但开通失败（已回滚）：%w", err)
	}
	return t, admin, nil
}

// provision 为新建租户创建初始管理员角色/权限/用户，返回初始凭据。
// 使用新租户自己的上下文（无 DisableScope、无 platform_admin 豁免），
// 使租户插件正确过滤、套餐约束正常生效。
func (s *TenantService) provision(t *model.Tenant) (*ProvisionedAdmin, error) {
	if s.db == nil {
		return nil, errors.New("tenant service db not configured")
	}
	pctx := tenant.WithContext(context.Background(), t.ID)
	roleSvc := NewRoleServiceWithDB(s.db)
	userSvc := NewUserServiceWithDB(s.db)
	permDAO := system.NewPermissionManageDAO(s.db)

	adminRole, err := roleSvc.CreateRoleContext(pctx, CreateRoleRequest{
		Name:        "管理员",
		Code:        "admin",
		DataScope:   string(authz.DataScopeAll),
		Description: "租户开通自动创建的管理员",
	})
	if err != nil {
		return nil, err
	}

	// 权限：绑套餐 → 套餐内权限码；未绑 → 全量。天然 ⊆ 套餐约束。
	var ids []uint
	if t.PackageID != nil {
		pkg, err := s.pkgDAO.GetByIDContext(pctx, *t.PackageID)
		if err != nil {
			return nil, err
		}
		for _, code := range pkg.PermissionCodes {
			p, err := permDAO.GetPermissionByCodeContext(pctx, code)
			if err != nil {
				return nil, err
			}
			ids = append(ids, p.ID)
		}
	} else {
		all, err := permDAO.ListAllContext(pctx)
		if err != nil {
			return nil, err
		}
		for _, p := range all {
			ids = append(ids, p.ID)
		}
	}
	if err := roleSvc.AssignPermissionsContext(pctx, adminRole.ID, AssignPermissionsRequest{PermissionIDs: ids}); err != nil {
		return nil, err
	}

	initialPwd, err := randomStrongPassword()
	if err != nil {
		return nil, err
	}
	adminUser, err := userSvc.CreateUserContext(pctx, CreateUserRequest{
		Username: "admin",
		Password: initialPwd,
		Nickname: "管理员",
	})
	if err != nil {
		return nil, err
	}
	if err := userSvc.AssignRolesContext(pctx, adminUser.ID, AssignRolesRequest{RoleIDs: []uint{adminRole.ID}}); err != nil {
		return nil, err
	}
	return &ProvisionedAdmin{Username: adminUser.Username, InitialPassword: initialPwd}, nil
}

// Delete 级联删除租户及其账号体系数据（users/roles/departments/posts/tenant_settings
// + join 表）与租户行。审计日志保留（不可变审计）；业务域租户数据（file/hermes/notice
// 等）不在此级联——平台先清业务数据或接受孤儿，注释以此为准。
func (s *TenantService) Delete(ctx context.Context, id uint) error {
	ctx = tenant.DisableScope(ctx)
	if id == 1 {
		return ErrDefaultTenantLocked
	}
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	if s.db == nil {
		return errors.New("tenant service db not configured")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", id).Delete(&model.TenantSetting{}).Error; err != nil {
			return err
		}
		var userIDs []uint
		if err := tx.Model(&model.User{}).Where("tenant_id = ?", id).Pluck("id", &userIDs).Error; err != nil {
			return err
		}
		if len(userIDs) > 0 {
			if err := tx.Where("user_id IN ?", userIDs).Delete(&model.UserPost{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id IN ?", userIDs).Delete(&model.UserRole{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&model.User{}).Error; err != nil {
			return err
		}
		var roleIDs []uint
		if err := tx.Model(&model.Role{}).Where("tenant_id = ?", id).Pluck("id", &roleIDs).Error; err != nil {
			return err
		}
		if len(roleIDs) > 0 {
			if err := tx.Where("role_id IN ?", roleIDs).Delete(&model.RolePermission{}).Error; err != nil {
				return err
			}
			if err := tx.Where("role_id IN ?", roleIDs).Delete(&model.RoleDataScopeDepartment{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&model.Role{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&model.Department{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&model.Post{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Tenant{}).Error
	})
}

// randomStrongPassword 生成满足 ValidatePasswordStrength 的初始密码
// （≥8 位，含大小写 + 数字）。
func randomStrongPassword() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	// 显式保证含大写/小写/数字，过密码强度校验。
	return "A" + string(b) + "a1", nil
}

func (s *TenantService) Update(ctx context.Context, id uint, req UpdateTenantRequest) (*model.Tenant, error) {
	ctx = tenant.DisableScope(ctx)
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrTenantNameRequired
		}
		t.Name = name
	}
	if req.Plan != nil {
		plan := strings.TrimSpace(*req.Plan)
		if plan == "" {
			plan = "free"
		}
		t.Plan = plan
	}
	if req.MaxUsers != nil {
		t.MaxUsers = *req.MaxUsers
	}
	if req.PackageID != nil {
		packageID, err := s.resolvePackageID(ctx, req.PackageID)
		if err != nil {
			return nil, err
		}
		t.PackageID = packageID
	}
	if req.Status != nil {
		// Default tenant (id=1) must stay enabled: disabling it would lock out
		// the platform admin and every default-tenant user at next refresh.
		if id == 1 && *req.Status != 1 {
			return nil, ErrDefaultTenantLocked
		}
		t.Status = *req.Status
	}
	if err := s.dao.UpdateContext(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TenantService) UserCount(ctx context.Context, tenantID uint) (int64, error) {
	// Count users of another tenant requires bypassing the actor tenant filter.
	ctx = tenant.DisableScope(ctx)
	return s.dao.CountUsersContext(ctx, tenantID)
}
