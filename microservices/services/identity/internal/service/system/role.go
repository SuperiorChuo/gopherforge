package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	"gorm.io/gorm"
)

type RoleService struct {
	roleDAO            systemdao.RoleDAO
	permissionCacheDAO *systemdao.PermissionCacheDAO
	// 租户套餐约束依赖（任一为 nil 时跳过约束，兼容旧的零值构造路径）。
	tenantDAO        *systemdao.TenantDAO
	tenantPackageDAO *systemdao.TenantPackageDAO
	permissionDAO    *systemdao.PermissionManageDAO
}

// NewRoleServiceWithDB builds a RoleService backed by an injected database handle.
func NewRoleServiceWithDB(db *gorm.DB) RoleService {
	return RoleService{
		roleDAO:            *systemdao.NewRoleDAO(db),
		permissionCacheDAO: systemdao.NewPermissionCacheDAO(db),
		tenantDAO:          systemdao.NewTenantDAO(db),
		tenantPackageDAO:   systemdao.NewTenantPackageDAO(db),
		permissionDAO:      systemdao.NewPermissionManageDAO(db),
	}
}

type RoleListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
}

type CreateRoleRequest struct {
	Name                   string `json:"name" binding:"required"`
	Code                   string `json:"code" binding:"required"`
	Description            string `json:"description"`
	DataScope              string `json:"data_scope"`
	DataScopeDepartmentIDs []uint `json:"data_scope_department_ids"`
}

type UpdateRoleRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	DataScope              string `json:"data_scope"`
	DataScopeDepartmentIDs []uint `json:"data_scope_department_ids"`
}

type AssignPermissionsRequest struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}

var (
	ErrRoleCodeAlreadyExists              = errors.New("role code already exists")
	ErrRoleNotFound                       = errors.New("role not found")
	ErrInvalidRoleDataScope               = errors.New("invalid data scope")
	ErrCustomDataScopeRequiresDepartments = errors.New("custom data scope requires department ids")
)

func (s *RoleService) GetRoleByIDContext(ctx context.Context, id uint) (*model.Role, error) {
	role, err := s.roleDAO.GetRoleByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	return role, nil
}

func (s *RoleService) GetRoleListContext(ctx context.Context, req RoleListRequest) ([]model.Role, int64, error) {
	return s.roleDAO.GetRoleListContext(ctx, req.PageRequest, req.Keyword)
}

func (s *RoleService) GetAllRolesContext(ctx context.Context) ([]model.Role, error) {
	return s.roleDAO.GetAllRolesContext(ctx)
}

func (s *RoleService) CreateRoleContext(ctx context.Context, req CreateRoleRequest) (*model.Role, error) {
	_, err := s.roleDAO.GetRoleByCodeContext(ctx, req.Code)
	if err == nil {
		return nil, ErrRoleCodeAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	dataScope := normalizeRoleDataScope(req.DataScope)
	departmentIDs := normalizeDepartmentIDs(req.DataScopeDepartmentIDs)
	if err := validateRoleDataScope(dataScope, departmentIDs); err != nil {
		return nil, err
	}

	tenantID := tenant.Normalize(tenant.FromContext(ctx))
	role := &model.Role{
		TenantID:               tenantID,
		Name:                   req.Name,
		Code:                   req.Code,
		Description:            req.Description,
		DataScope:              dataScope,
		DataScopeDepartmentIDs: departmentIDs,
	}

	if err := s.roleDAO.CreateRoleContext(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}

func (s *RoleService) UpdateRoleContext(ctx context.Context, id uint, req UpdateRoleRequest) (*model.Role, error) {
	role, err := s.roleDAO.GetRoleByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Description != "" {
		role.Description = req.Description
	}
	if req.DataScope != "" {
		role.DataScope = normalizeRoleDataScope(req.DataScope)
	}
	if role.DataScope == "" {
		role.DataScope = string(authz.DataScopeSelf)
	}
	if req.DataScopeDepartmentIDs != nil {
		role.DataScopeDepartmentIDs = normalizeDepartmentIDs(req.DataScopeDepartmentIDs)
	}
	if role.DataScope != string(authz.DataScopeCustom) {
		role.DataScopeDepartmentIDs = nil
	}
	if err := validateRoleDataScope(role.DataScope, role.DataScopeDepartmentIDs); err != nil {
		return nil, err
	}

	if err := s.roleDAO.UpdateRoleContext(ctx, role); err != nil {
		return nil, err
	}

	if err := InvalidatePermissionCacheByRolesContext(ctx, s.permissionCacheDAO, id); err != nil {
		return nil, err
	}

	return role, nil
}

func (s *RoleService) DeleteRoleContext(ctx context.Context, id uint) error {
	if _, err := s.roleDAO.GetRoleByIDContext(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	if err := InvalidatePermissionCacheByRolesContext(ctx, s.permissionCacheDAO, id); err != nil {
		return err
	}

	return s.roleDAO.DeleteRoleContext(ctx, id)
}

func (s *RoleService) AssignPermissionsContext(ctx context.Context, roleID uint, req AssignPermissionsRequest) error {
	role, err := s.roleDAO.GetRoleByIDContext(ctx, roleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	// 租户套餐约束：租户内角色可分配的权限必须 ⊆ 套餐 permission_codes。
	if err := s.enforceTenantPackageContext(ctx, role.TenantID, req.PermissionIDs); err != nil {
		return err
	}

	if err := s.roleDAO.AssignPermissionsContext(ctx, roleID, req.PermissionIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheByRolesContext(ctx, s.permissionCacheDAO, roleID)
}

// isPlatformAdminContext 读取认证中间件写入的平台管理员标志（middleware/auth.go）。
func isPlatformAdminContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value("platform_admin").(bool)
	return v
}

// enforceTenantPackageContext 校验角色所属租户绑定的套餐是否允许分配指定权限。
// 豁免：平台管理员/超管（platform_admin 上下文标志）、未绑定套餐的租户、旧构造路径（约束依赖未注入）。
// 语义（M1）：套餐改小后不回收已有角色的越界权限，只拦截新的分配请求（挡新分配）。
func (s *RoleService) enforceTenantPackageContext(ctx context.Context, roleTenantID uint, permissionIDs []uint) error {
	if s.tenantDAO == nil || s.tenantPackageDAO == nil || s.permissionDAO == nil {
		return nil
	}
	if isPlatformAdminContext(ctx) {
		return nil
	}
	if len(permissionIDs) == 0 {
		return nil // 清空权限总是允许
	}
	// 平台级目录读绕过行级租户过滤（tenants / tenant_packages 无 tenant_id 列）。
	qctx := tenant.DisableScope(ctx)
	t, err := s.tenantDAO.GetByIDContext(qctx, tenant.Normalize(roleTenantID))
	if err != nil {
		// 单租户旧库无 tenants 行时不启用约束。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if t.PackageID == nil || *t.PackageID == 0 {
		return nil // 未绑定套餐 = 不限
	}
	pkg, err := s.tenantPackageDAO.GetByIDContext(qctx, *t.PackageID)
	if err != nil {
		// 绑定的套餐已不存在（删除有绑定守卫，此处防御式放行）。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	codes, err := s.permissionDAO.FindCodesByIDsContext(qctx, permissionIDs)
	if err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(pkg.PermissionCodes))
	for _, c := range pkg.PermissionCodes {
		allowed[c] = struct{}{}
	}
	var exceeded []string
	for _, c := range codes {
		if _, ok := allowed[c]; !ok {
			exceeded = append(exceeded, c)
		}
	}
	if len(exceeded) > 0 {
		return &PermissionsExceedPackageError{Codes: exceeded}
	}
	return nil
}

func normalizeRoleDataScope(dataScope string) string {
	if dataScope == "" {
		return string(authz.DataScopeSelf)
	}
	return dataScope
}

func validateRoleDataScope(dataScope string, departmentIDs []uint) error {
	switch authz.DataScope(dataScope) {
	case authz.DataScopeAll,
		authz.DataScopeDepartment,
		authz.DataScopeDepartmentTree,
		authz.DataScopeSelf,
		authz.DataScopeCustom,
		authz.DataScopeNone:
	default:
		return ErrInvalidRoleDataScope
	}

	if dataScope == string(authz.DataScopeCustom) && len(departmentIDs) == 0 {
		return ErrCustomDataScopeRequiresDepartments
	}
	return nil
}

func normalizeDepartmentIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}

	normalized := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}
