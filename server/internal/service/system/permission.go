package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type PermissionService struct {
	permissionDAO systemdao.PermissionManageDAO
}

type PermissionListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Type    *int8  `json:"type" form:"type"`
}

type CreatePermissionRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Type        int8   `json:"type" binding:"required"`
	Path        string `json:"path"`
	Method      string `json:"method"`
	ParentID    uint   `json:"parent_id"`
}

type UpdatePermissionRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Path        string  `json:"path"`
	Method      string  `json:"method"`
	ParentID    uint    `json:"parent_id"`
}

var (
	ErrPermissionCodeAlreadyExists  = errors.New("permission code already exists")
	ErrPermissionNotFound           = errors.New("permission not found")
	ErrParentPermissionNotFound     = errors.New("parent permission not found")
	ErrPermissionParentIsDescendant = errors.New("cannot set parent to descendant")
)

// Deprecated: use GetPermissionByIDContext instead.
func (s *PermissionService) GetPermissionByID(id uint) (*model.Permission, error) {
	return s.GetPermissionByIDContext(context.Background(), id)
}

func (s *PermissionService) GetPermissionByIDContext(ctx context.Context, id uint) (*model.Permission, error) {
	permission, err := s.permissionDAO.GetPermissionByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPermissionNotFound
		}
		return nil, err
	}
	return permission, nil
}

// Deprecated: use GetPermissionListContext instead.
func (s *PermissionService) GetPermissionList(req PermissionListRequest) ([]model.Permission, int64, error) {
	return s.GetPermissionListContext(context.Background(), req)
}

func (s *PermissionService) GetPermissionListContext(ctx context.Context, req PermissionListRequest) ([]model.Permission, int64, error) {
	return s.permissionDAO.GetPermissionListContext(ctx, req.PageRequest, req.Keyword, req.Type)
}

// Deprecated: use GetPermissionTreeContext instead.
func (s *PermissionService) GetPermissionTree() ([]model.Permission, error) {
	return s.GetPermissionTreeContext(context.Background())
}

func (s *PermissionService) GetPermissionTreeContext(ctx context.Context) ([]model.Permission, error) {
	return s.permissionDAO.GetPermissionTreeContext(ctx)
}

// Deprecated: use CreatePermissionContext instead.
func (s *PermissionService) CreatePermission(req CreatePermissionRequest) (*model.Permission, error) {
	return s.CreatePermissionContext(context.Background(), req)
}

func (s *PermissionService) CreatePermissionContext(ctx context.Context, req CreatePermissionRequest) (*model.Permission, error) {
	_, err := s.permissionDAO.GetPermissionByCodeContext(ctx, req.Code)
	if err == nil {
		return nil, ErrPermissionCodeAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if req.ParentID > 0 {
		_, err := s.permissionDAO.GetPermissionByIDContext(ctx, req.ParentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrParentPermissionNotFound
			}
			return nil, err
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

	if err := s.permissionDAO.CreatePermissionContext(ctx, permission); err != nil {
		return nil, err
	}

	return permission, nil
}

// Deprecated: use UpdatePermissionContext instead.
func (s *PermissionService) UpdatePermission(id uint, req UpdatePermissionRequest) (*model.Permission, error) {
	return s.UpdatePermissionContext(context.Background(), id, req)
}

func (s *PermissionService) UpdatePermissionContext(ctx context.Context, id uint, req UpdatePermissionRequest) (*model.Permission, error) {
	permission, err := s.permissionDAO.GetPermissionByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPermissionNotFound
		}
		return nil, err
	}

	if req.ParentID > 0 && req.ParentID != permission.ParentID {
		descendant, err := isDescendantContext(ctx, s.permissionDAO, id, req.ParentID)
		if err != nil {
			return nil, err
		}
		if descendant {
			return nil, ErrPermissionParentIsDescendant
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

	if err := s.permissionDAO.UpdatePermissionContext(ctx, permission); err != nil {
		return nil, err
	}

	if err := InvalidatePermissionCacheByPermissionsContext(ctx, id); err != nil {
		return nil, err
	}

	return permission, nil
}

// Deprecated: use DeletePermissionContext instead.
func (s *PermissionService) DeletePermission(id uint) error {
	return s.DeletePermissionContext(context.Background(), id)
}

func (s *PermissionService) DeletePermissionContext(ctx context.Context, id uint) error {
	if _, err := s.permissionDAO.GetPermissionByIDContext(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPermissionNotFound
		}
		return err
	}

	if err := InvalidatePermissionCacheByPermissionsContext(ctx, id); err != nil {
		return err
	}

	return s.permissionDAO.DeletePermissionContext(ctx, id)
}

func isDescendantContext(ctx context.Context, permissionDAO systemdao.PermissionManageDAO, ancestorID, targetID uint) (bool, error) {
	if targetID == 0 {
		return false, nil
	}
	target, err := permissionDAO.GetPermissionByIDContext(ctx, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if target.ParentID == ancestorID {
		return true, nil
	}
	return isDescendantContext(ctx, permissionDAO, ancestorID, target.ParentID)
}
