package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// RoleService 角色服务
type RoleService struct {
	roleDAO system.RoleDAO
}

// RoleListRequest 角色列表请求
type RoleListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
}

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name                   string `json:"name" binding:"required"`
	Code                   string `json:"code" binding:"required"`
	Description            string `json:"description"`
	DataScope              string `json:"data_scope"`
	DataScopeDepartmentIDs []uint `json:"data_scope_department_ids"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	DataScope              string `json:"data_scope"`
	DataScopeDepartmentIDs []uint `json:"data_scope_department_ids"`
}

// AssignPermissionsRequest 分配权限请求
type AssignPermissionsRequest struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}

// GetRoleByID 根据ID获取角色
func (s *RoleService) GetRoleByID(id uint) (*model.Role, error) {
	return s.roleDAO.GetRoleByID(id)
}

// GetRoleList 获取角色列表
func (s *RoleService) GetRoleList(req RoleListRequest) ([]model.Role, int64, error) {
	return s.roleDAO.GetRoleList(req.PageRequest, req.Keyword)
}

// GetAllRoles 获取所有角色
func (s *RoleService) GetAllRoles() ([]model.Role, error) {
	return s.roleDAO.GetAllRoles()
}

// CreateRole 创建角色
func (s *RoleService) CreateRole(req CreateRoleRequest) (*model.Role, error) {
	// 检查角色代码是否已存在
	_, err := s.roleDAO.GetRoleByCode(req.Code)
	if err == nil {
		return nil, errors.New("role code already exists")
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

	if err := s.roleDAO.CreateRole(role); err != nil {
		return nil, err
	}

	return role, nil
}

// UpdateRole 更新角色
func (s *RoleService) UpdateRole(id uint, req UpdateRoleRequest) (*model.Role, error) {
	role, err := s.roleDAO.GetRoleByID(id)
	if err != nil {
		return nil, errors.New("role not found")
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

	if err := s.roleDAO.UpdateRole(role); err != nil {
		return nil, err
	}

	if err := InvalidatePermissionCacheByRoles(id); err != nil {
		return nil, err
	}

	return role, nil
}

// DeleteRole 删除角色
func (s *RoleService) DeleteRole(id uint) error {
	if _, err := s.roleDAO.GetRoleByID(id); err != nil {
		return errors.New("role not found")
	}

	if err := InvalidatePermissionCacheByRoles(id); err != nil {
		return err
	}

	return s.roleDAO.DeleteRole(id)
}

// AssignPermissions 分配权限
func (s *RoleService) AssignPermissions(roleID uint, req AssignPermissionsRequest) error {
	// 检查角色是否存在
	_, err := s.roleDAO.GetRoleByID(roleID)
	if err != nil {
		return errors.New("role not found")
	}

	if err := s.roleDAO.AssignPermissions(roleID, req.PermissionIDs); err != nil {
		return err
	}

	return InvalidatePermissionCacheByRoles(roleID)
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
		return errors.New("invalid data scope")
	}

	if dataScope == string(authz.DataScopeCustom) && len(departmentIDs) == 0 {
		return errors.New("custom data scope requires department ids")
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
