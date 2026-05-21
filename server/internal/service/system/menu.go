package system

import (
	"context"
	"errors"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/gorm"
)

type MenuService struct {
	menuDAO systemdao.MenuDAO
}

type MenuListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Status  *int8  `json:"status" form:"status"`
}

type CreateMenuRequest struct {
	Name          string `json:"name" binding:"required"`
	Title         string `json:"title" binding:"required"`
	Icon          string `json:"icon"`
	Path          string `json:"path"`
	Component     string `json:"component"`
	ParentID      uint   `json:"parent_id"`
	Sort          int    `json:"sort"`
	Status        int8   `json:"status"`
	Hidden        int8   `json:"hidden"`
	Permission    string `json:"permission"`
	PermissionIDs []uint `json:"permission_ids"`
}

type UpdateMenuRequest struct {
	Name          string `json:"name"`
	Title         string `json:"title"`
	Icon          string `json:"icon"`
	Path          string `json:"path"`
	Component     string `json:"component"`
	ParentID      uint   `json:"parent_id"`
	Sort          int    `json:"sort"`
	Status        *int8  `json:"status"`
	Hidden        *int8  `json:"hidden"`
	Permission    string `json:"permission"`
	PermissionIDs []uint `json:"permission_ids"`
}

var (
	ErrMenuNotFound           = errors.New("menu not found")
	ErrParentMenuNotFound     = errors.New("parent menu not found")
	ErrMenuParentIsDescendant = errors.New("cannot set parent to descendant")
	ErrMenuHasChildren        = errors.New("cannot delete menu with children")
)

// Deprecated: use GetMenuByIDContext instead.
func (s *MenuService) GetMenuByID(id uint) (*model.Menu, error) {
	return s.GetMenuByIDContext(context.Background(), id)
}

func (s *MenuService) GetMenuByIDContext(ctx context.Context, id uint) (*model.Menu, error) {
	menu, err := s.menuDAO.GetMenuByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuNotFound
		}
		return nil, err
	}
	return menu, nil
}

// Deprecated: use GetMenuListContext instead.
func (s *MenuService) GetMenuList(req MenuListRequest) ([]model.Menu, int64, error) {
	return s.GetMenuListContext(context.Background(), req)
}

func (s *MenuService) GetMenuListContext(ctx context.Context, req MenuListRequest) ([]model.Menu, int64, error) {
	return s.menuDAO.GetMenuListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

// Deprecated: use GetMenuTreeContext instead.
func (s *MenuService) GetMenuTree(status *int8) ([]model.Menu, error) {
	return s.GetMenuTreeContext(context.Background(), status)
}

func (s *MenuService) GetMenuTreeContext(ctx context.Context, status *int8) ([]model.Menu, error) {
	return s.menuDAO.GetMenuTreeContext(ctx, status)
}

// Deprecated: use CreateMenuContext instead.
func (s *MenuService) CreateMenu(req CreateMenuRequest) (*model.Menu, error) {
	return s.CreateMenuContext(context.Background(), req)
}

func (s *MenuService) CreateMenuContext(ctx context.Context, req CreateMenuRequest) (*model.Menu, error) {
	if req.ParentID > 0 {
		_, err := s.menuDAO.GetMenuByIDContext(ctx, req.ParentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrParentMenuNotFound
			}
			return nil, err
		}
	}

	menu := &model.Menu{
		Name:       req.Name,
		Title:      req.Title,
		Icon:       req.Icon,
		Path:       req.Path,
		Component:  req.Component,
		ParentID:   req.ParentID,
		Sort:       req.Sort,
		Status:     req.Status,
		Hidden:     req.Hidden,
		Permission: req.Permission,
	}

	if menu.Status == 0 {
		menu.Status = 1
	}

	if err := s.menuDAO.CreateMenuContext(ctx, menu); err != nil {
		return nil, err
	}

	if len(req.PermissionIDs) > 0 {
		if err := s.menuDAO.AssignPermissionsContext(ctx, menu.ID, req.PermissionIDs); err != nil {
			return nil, err
		}
	}

	if req.Permission != "" || len(req.PermissionIDs) > 0 {
		if err := InvalidatePermissionCacheAllContext(ctx); err != nil {
			return nil, err
		}
	}

	return menu, nil
}

// Deprecated: use UpdateMenuContext instead.
func (s *MenuService) UpdateMenu(id uint, req UpdateMenuRequest) (*model.Menu, error) {
	return s.UpdateMenuContext(context.Background(), id, req)
}

func (s *MenuService) UpdateMenuContext(ctx context.Context, id uint, req UpdateMenuRequest) (*model.Menu, error) {
	menu, err := s.menuDAO.GetMenuByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuNotFound
		}
		return nil, err
	}

	if req.ParentID > 0 && req.ParentID != menu.ParentID {
		descendant, err := isMenuDescendantContext(ctx, &s.menuDAO, id, req.ParentID)
		if err != nil {
			return nil, err
		}
		if descendant {
			return nil, ErrMenuParentIsDescendant
		}
		menu.ParentID = req.ParentID
	}

	if req.Name != "" {
		menu.Name = req.Name
	}
	if req.Title != "" {
		menu.Title = req.Title
	}
	if req.Icon != "" {
		menu.Icon = req.Icon
	}
	if req.Path != "" {
		menu.Path = req.Path
	}
	if req.Component != "" {
		menu.Component = req.Component
	}
	if req.Sort > 0 {
		menu.Sort = req.Sort
	}
	if req.Status != nil {
		menu.Status = *req.Status
	}
	if req.Hidden != nil {
		menu.Hidden = *req.Hidden
	}
	if req.Permission != "" {
		menu.Permission = req.Permission
	}

	if err := s.menuDAO.UpdateMenuContext(ctx, menu); err != nil {
		return nil, err
	}

	if req.PermissionIDs != nil {
		if err := s.menuDAO.AssignPermissionsContext(ctx, menu.ID, req.PermissionIDs); err != nil {
			return nil, err
		}
	}

	if err := InvalidatePermissionCacheAllContext(ctx); err != nil {
		return nil, err
	}

	return menu, nil
}

// Deprecated: use DeleteMenuContext instead.
func (s *MenuService) DeleteMenu(id uint) error {
	return s.DeleteMenuContext(context.Background(), id)
}

func (s *MenuService) DeleteMenuContext(ctx context.Context, id uint) error {
	if _, err := s.menuDAO.GetMenuByIDContext(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMenuNotFound
		}
		return err
	}

	if err := s.menuDAO.DeleteMenuContext(ctx, id); err != nil {
		if errors.Is(err, systemdao.ErrMenuHasChildren) {
			return ErrMenuHasChildren
		}
		return err
	}

	return InvalidatePermissionCacheAllContext(ctx)
}

func isMenuDescendantContext(ctx context.Context, dao *systemdao.MenuDAO, ancestorID, targetID uint) (bool, error) {
	if targetID == 0 {
		return false, nil
	}
	target, err := dao.GetMenuByIDContext(ctx, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if target.ParentID == ancestorID {
		return true, nil
	}
	return isMenuDescendantContext(ctx, dao, ancestorID, target.ParentID)
}
