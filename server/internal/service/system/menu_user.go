package system

import (
	"github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
)

// MenuUserService 用户菜单服务
type MenuUserService struct {
	menuDAO       system.MenuDAO
	permissionDAO auth.PermissionDAO
}

// GetUserMenuTree 获取当前用户的菜单树
func (s *MenuUserService) GetUserMenuTree(userID uint) ([]model.Menu, error) {
	// 1. 获取所有通过开启状态的菜单树 (status=1)
	// 使用 nil 表示获取所有状态为 1 的菜单（因为 GetMenuTree 内部逻辑是 status != nil 才筛选，
	// 但我们需要明确只获取启用的菜单，MenuDAO.GetMenuTree 接受 *int8）
	status := int8(1)
	allMenus, err := s.menuDAO.GetMenuTree(&status)
	if err != nil {
		return nil, err
	}

	// 2. 获取用户的所有权限代码
	permissionCodes, err := s.permissionDAO.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	// 将权限代码转换为 map，方便快速查找
	permissionMap := make(map[string]bool)
	for _, code := range permissionCodes {
		permissionMap[code] = true
	}

	// 3. 递归过滤菜单
	// 规则：
	// - 如果菜单需要权限，用户必须有该权限
	// - 如果菜单不需要权限，默认可见
	// - 即使父菜单不可见（无权限），如果子菜单可见，父菜单也应该保留（作为目录显示）
	// - 处理 Hidden 属性（前端路由需要）
	return s.filterMenusByPermission(allMenus, permissionMap), nil
}

// filterMenusByPermission 根据权限过滤菜单
func (s *MenuUserService) filterMenusByPermission(menus []model.Menu, permissionMap map[string]bool) []model.Menu {
	var filtered []model.Menu

	for _, menu := range menus {
		// 递归处理子菜单
		if len(menu.Children) > 0 {
			menu.Children = s.filterMenusByPermission(menu.Children, permissionMap)
		}

		// 判断当前菜单是否可见
		hasPermission := false
		if menu.Permission == "" {
			// 没有设置权限，默认可见
			hasPermission = true
		} else if permissionMap[menu.Permission] {
			// 用户拥有该权限
			hasPermission = true
		}

		// 决定是否保留当前菜单：
		// 1. 用户有直接权限
		// 2. 或者用户有子菜单的权限（这种情况下，当前菜单作为容器显示）
		if hasPermission || len(menu.Children) > 0 {
			filtered = append(filtered, menu)
		}
	}

	return filtered
}
