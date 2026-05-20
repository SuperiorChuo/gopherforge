package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// MenuService 菜单服务
type MenuService struct {
	menuDAO system.MenuDAO
}

// MenuListRequest 菜单列表请求
type MenuListRequest struct {
	pagination.PageRequest
	Keyword string `json:"keyword" form:"keyword"`
	Status  *int8  `json:"status" form:"status"`
}

// CreateMenuRequest 创建菜单请求
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
	Permission    string `json:"permission"`     // 关联的权限代码
	PermissionIDs []uint `json:"permission_ids"` // 关联的权限ID列表（通过 menu_permissions 表）
}

// UpdateMenuRequest 更新菜单请求
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
	Permission    string `json:"permission"`     // 关联的权限代码
	PermissionIDs []uint `json:"permission_ids"` // 关联的权限ID列表（通过 menu_permissions 表）
}

// GetMenuByID 根据ID获取菜单
func (s *MenuService) GetMenuByID(id uint) (*model.Menu, error) {
	return s.menuDAO.GetMenuByID(id)
}

// GetMenuList 获取菜单列表
func (s *MenuService) GetMenuList(req MenuListRequest) ([]model.Menu, int64, error) {
	return s.menuDAO.GetMenuList(req.PageRequest, req.Keyword, req.Status)
}

// GetMenuTree 获取菜单树
func (s *MenuService) GetMenuTree(status *int8) ([]model.Menu, error) {
	return s.menuDAO.GetMenuTree(status)
}

// CreateMenu 创建菜单
func (s *MenuService) CreateMenu(req CreateMenuRequest) (*model.Menu, error) {
	// 如果指定了父菜单，检查父菜单是否存在
	if req.ParentID > 0 {
		_, err := s.menuDAO.GetMenuByID(req.ParentID)
		if err != nil {
			return nil, errors.New("parent menu not found")
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

	if err := s.menuDAO.CreateMenu(menu); err != nil {
		return nil, err
	}

	// 如果指定了权限ID列表，创建菜单权限关联
	if len(req.PermissionIDs) > 0 {
		if err := s.menuDAO.AssignPermissions(menu.ID, req.PermissionIDs); err != nil {
			return nil, err
		}
	}

	if req.Permission != "" || len(req.PermissionIDs) > 0 {
		if err := InvalidatePermissionCacheAll(); err != nil {
			return nil, err
		}
	}

	return menu, nil
}

// UpdateMenu 更新菜单
func (s *MenuService) UpdateMenu(id uint, req UpdateMenuRequest) (*model.Menu, error) {
	menu, err := s.menuDAO.GetMenuByID(id)
	if err != nil {
		return nil, errors.New("menu not found")
	}

	// 如果更新父菜单，检查是否会造成循环引用
	if req.ParentID > 0 && req.ParentID != menu.ParentID {
		// 检查新父菜单是否是当前菜单的子菜单
		if isMenuDescendant(&s.menuDAO, id, req.ParentID) {
			return nil, errors.New("cannot set parent to descendant")
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

	if err := s.menuDAO.UpdateMenu(menu); err != nil {
		return nil, err
	}

	// 如果指定了权限ID列表，更新菜单权限关联
	if req.PermissionIDs != nil {
		if err := s.menuDAO.AssignPermissions(menu.ID, req.PermissionIDs); err != nil {
			return nil, err
		}
	}

	if err := InvalidatePermissionCacheAll(); err != nil {
		return nil, err
	}

	return menu, nil
}

// DeleteMenu 删除菜单
func (s *MenuService) DeleteMenu(id uint) error {
	if _, err := s.menuDAO.GetMenuByID(id); err != nil {
		return errors.New("menu not found")
	}

	if err := s.menuDAO.DeleteMenu(id); err != nil {
		return err
	}

	return InvalidatePermissionCacheAll()
}

// isMenuDescendant 检查target是否是ancestor的后代
func isMenuDescendant(dao *system.MenuDAO, ancestorID, targetID uint) bool {
	if targetID == 0 {
		return false
	}
	target, err := dao.GetMenuByID(targetID)
	if err != nil {
		return false
	}
	if target.ParentID == ancestorID {
		return true
	}
	return isMenuDescendant(dao, ancestorID, target.ParentID)
}
