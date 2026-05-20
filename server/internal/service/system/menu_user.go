package system

import (
	"context"

	"github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
)

// MenuUserService builds user menu trees.
type MenuUserService struct {
	menuDAO       system.MenuDAO
	permissionDAO auth.PermissionDAO
}

// GetUserMenuTree returns the current user's menu tree.
func (s *MenuUserService) GetUserMenuTree(userID uint) ([]model.Menu, error) {
	return s.GetUserMenuTreeContext(context.Background(), userID)
}

func (s *MenuUserService) GetUserMenuTreeContext(ctx context.Context, userID uint) ([]model.Menu, error) {
	// Load enabled menus only.
	status := int8(1)
	allMenus, err := s.menuDAO.GetMenuTreeContext(ctx, &status)
	if err != nil {
		return nil, err
	}

	// Load all permission codes for the user.
	permissionCodes, err := s.permissionDAO.GetUserPermissionsContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert permission codes to a map for fast lookup.
	permissionMap := make(map[string]bool)
	for _, code := range permissionCodes {
		permissionMap[code] = true
	}

	// Recursively keep directly visible menus and parent containers with visible children.
	return s.filterMenusByPermission(allMenus, permissionMap), nil
}

// filterMenusByPermission filters menus by permission.
func (s *MenuUserService) filterMenusByPermission(menus []model.Menu, permissionMap map[string]bool) []model.Menu {
	var filtered []model.Menu

	for _, menu := range menus {
		// Recursively process child menus.
		if len(menu.Children) > 0 {
			menu.Children = s.filterMenusByPermission(menu.Children, permissionMap)
		}

		// Check whether the current menu is visible.
		hasPermission := false
		if menu.Permission == "" {
			// Menus without explicit permissions are visible by default.
			hasPermission = true
		} else if permissionMap[menu.Permission] {
			// The user has the required permission.
			hasPermission = true
		}

		// Keep menus with direct permission or visible children.
		if hasPermission || len(menu.Children) > 0 {
			filtered = append(filtered, menu)
		}
	}

	return filtered
}
