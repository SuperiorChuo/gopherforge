package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// PermissionService 权限服务
type PermissionService struct {
	permissionDAO system.PermissionManageDAO
}

// PermissionListRequest 权限列表请求
type PermissionListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Type    *int8  `json:"type" form:"type"` // 1菜单，2按钮
}

// CreatePermissionRequest 创建权限请求
type CreatePermissionRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Type        int8   `json:"type" binding:"required"` // 1菜单，2按钮
	Path        string `json:"path"`
	Method      string `json:"method"`
	ParentID    uint   `json:"parent_id"`
}

// UpdatePermissionRequest 更新权限请求
type UpdatePermissionRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Path        string  `json:"path"`
	Method      string  `json:"method"`
	ParentID    uint    `json:"parent_id"`
}

// GetPermissionByID 根据ID获取权限
func (s *PermissionService) GetPermissionByID(id uint) (*model.Permission, error) {
	return s.permissionDAO.GetPermissionByID(id)
}

// GetPermissionList 获取权限列表
func (s *PermissionService) GetPermissionList(req PermissionListRequest) ([]model.Permission, int64, error) {
	return s.permissionDAO.GetPermissionList(req.PageRequest, req.Keyword, req.Type)
}

// GetPermissionTree 获取权限树
func (s *PermissionService) GetPermissionTree() ([]model.Permission, error) {
	return s.permissionDAO.GetPermissionTree()
}

// CreatePermission 创建权限
func (s *PermissionService) CreatePermission(req CreatePermissionRequest) (*model.Permission, error) {
	// 检查权限代码是否已存在
	_, err := s.permissionDAO.GetPermissionByCode(req.Code)
	if err == nil {
		return nil, errors.New("permission code already exists")
	}

	// 如果指定了父权限，检查父权限是否存在
	if req.ParentID > 0 {
		_, err := s.permissionDAO.GetPermissionByID(req.ParentID)
		if err != nil {
			return nil, errors.New("parent permission not found")
		}
	}

	permission := &model.Permission{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Type:        req.Type,
		Path:        req.Path,
		Method:      req.Method,
		ParentID:    req.ParentID,
	}

	if err := s.permissionDAO.CreatePermission(permission); err != nil {
		return nil, err
	}

	return permission, nil
}

// UpdatePermission 更新权限
func (s *PermissionService) UpdatePermission(id uint, req UpdatePermissionRequest) (*model.Permission, error) {
	permission, err := s.permissionDAO.GetPermissionByID(id)
	if err != nil {
		return nil, errors.New("permission not found")
	}

	// 如果更新父权限，检查是否会造成循环引用
	if req.ParentID > 0 && req.ParentID != permission.ParentID {
		// 检查新父权限是否是当前权限的子权限
		if isDescendant(s.permissionDAO, id, req.ParentID) {
			return nil, errors.New("cannot set parent to descendant")
		}
		permission.ParentID = req.ParentID
	}

	if req.Name != "" {
		permission.Name = req.Name
	}
	if req.Description != nil {
		permission.Description = *req.Description
	}
	if req.Path != "" {
		permission.Path = req.Path
	}
	if req.Method != "" {
		permission.Method = req.Method
	}

	if err := s.permissionDAO.UpdatePermission(permission); err != nil {
		return nil, err
	}

	if err := InvalidatePermissionCacheByPermissions(id); err != nil {
		return nil, err
	}

	return permission, nil
}

// DeletePermission 删除权限
func (s *PermissionService) DeletePermission(id uint) error {
	if _, err := s.permissionDAO.GetPermissionByID(id); err != nil {
		return errors.New("permission not found")
	}

	if err := InvalidatePermissionCacheByPermissions(id); err != nil {
		return err
	}

	return s.permissionDAO.DeletePermission(id)
}

// isDescendant 检查target是否是ancestor的后代
func isDescendant(permissionDAO system.PermissionManageDAO, ancestorID, targetID uint) bool {
	if targetID == 0 {
		return false
	}
	target, err := permissionDAO.GetPermissionByID(targetID)
	if err != nil {
		return false
	}
	if target.ParentID == ancestorID {
		return true
	}
	return isDescendant(permissionDAO, ancestorID, target.ParentID)
}
