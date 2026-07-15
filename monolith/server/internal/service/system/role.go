package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type RoleService struct {
	roleDAO            systemdao.RoleDAO
	permissionCacheDAO *systemdao.PermissionCacheDAO
}

// NewRoleServiceWithDB builds a RoleService backed by an injected database handle.
func NewRoleServiceWithDB(db *gorm.DB) RoleService {
	return RoleService{
		roleDAO:            *systemdao.NewRoleDAO(db),
		permissionCacheDAO: systemdao.NewPermissionCacheDAO(db),
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

	role := &model.Role{
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
	_, err := s.roleDAO.GetRoleByIDContext(ctx, roleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	if err := s.roleDAO.AssignPermissionsContext(ctx, roleID, req.PermissionIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheByRolesContext(ctx, s.permissionCacheDAO, roleID)
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
