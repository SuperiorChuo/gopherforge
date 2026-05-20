package system

import (
	"errors"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// MenuDAO 菜单数据访问对象
type MenuDAO struct{}

// GetMenuByID 根据ID获取菜单
func (d *MenuDAO) GetMenuByID(id uint) (*model.Menu, error) {
	var menu model.Menu
	result := database.DB.First(&menu, id)
	return &menu, result.Error
}

// GetMenuList 获取菜单列表（分页）
func (d *MenuDAO) GetMenuList(req pagination.PageRequest, keyword string, status *int8) ([]model.Menu, int64, error) {
	var menus []model.Menu
	var total int64

	query := database.DB.Model(&model.Menu{})

	// 关键词搜索
	if keyword != "" {
		query = query.Where("name LIKE ? OR title LIKE ? OR path LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, sort ASC, created_at ASC").
		Find(&menus)

	return menus, total, result.Error
}

// GetMenuTree 获取菜单树
func (d *MenuDAO) GetMenuTree(status *int8) ([]model.Menu, error) {
	query := database.DB.Model(&model.Menu{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var menus []model.Menu
	result := query.Order("parent_id ASC, sort ASC, created_at ASC").Find(&menus)
	if result.Error != nil {
		return nil, result.Error
	}

	// 构建树形结构
	return buildMenuTree(menus, 0), nil
}

// buildMenuTree 构建菜单树
func buildMenuTree(menus []model.Menu, parentID uint) []model.Menu {
	var tree []model.Menu
	for i := range menus {
		if menus[i].ParentID == parentID {
			children := buildMenuTree(menus, menus[i].ID)
			// 确保 children 字段始终存在（即使是空数组）
			if children == nil {
				menus[i].Children = []model.Menu{}
			} else {
				menus[i].Children = children
			}
			tree = append(tree, menus[i])
		}
	}
	return tree
}

// CreateMenu 创建菜单
func (d *MenuDAO) CreateMenu(menu *model.Menu) error {
	return database.DB.Create(menu).Error
}

// UpdateMenu 更新菜单
func (d *MenuDAO) UpdateMenu(menu *model.Menu) error {
	return database.DB.Save(menu).Error
}

// DeleteMenu 删除菜单
func (d *MenuDAO) DeleteMenu(id uint) error {
	// 检查是否有子菜单
	var count int64
	database.DB.Model(&model.Menu{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("cannot delete menu with children")
	}

	// 先删除菜单权限关联
	database.DB.Where("menu_id = ?", id).Delete(&model.MenuPermission{})

	// 再删除菜单
	return database.DB.Delete(&model.Menu{}, id).Error
}

// AssignPermissions 为菜单分配权限
func (d *MenuDAO) AssignPermissions(menuID uint, permissionIDs []uint) error {
	// 先删除原有关联
	database.DB.Where("menu_id = ?", menuID).Delete(&model.MenuPermission{})

	// 添加新关联
	for _, permissionID := range permissionIDs {
		menuPermission := model.MenuPermission{
			MenuID:       menuID,
			PermissionID: permissionID,
		}
		if err := database.DB.Create(&menuPermission).Error; err != nil {
			return err
		}
	}

	return nil
}
