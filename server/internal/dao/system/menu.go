package system

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type MenuDAO struct{}

func (d *MenuDAO) GetMenuByID(id uint) (*model.Menu, error) {
	return d.GetMenuByIDContext(context.Background(), id)
}

func (d *MenuDAO) GetMenuByIDContext(ctx context.Context, id uint) (*model.Menu, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var menu model.Menu
	result := database.DB.WithContext(ctx).First(&menu, id)
	return &menu, result.Error
}

func (d *MenuDAO) GetMenuList(req pagination.PageRequest, keyword string, status *int8) ([]model.Menu, int64, error) {
	return d.GetMenuListContext(context.Background(), req, keyword, status)
}

func (d *MenuDAO) GetMenuListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Menu, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var menus []model.Menu
	var total int64

	query := database.DB.WithContext(ctx).Model(&model.Menu{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR title LIKE ? OR path LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, sort ASC, created_at ASC").
		Find(&menus)

	return menus, total, result.Error
}

func (d *MenuDAO) GetMenuTree(status *int8) ([]model.Menu, error) {
	return d.GetMenuTreeContext(context.Background(), status)
}

func (d *MenuDAO) GetMenuTreeContext(ctx context.Context, status *int8) ([]model.Menu, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := database.DB.WithContext(ctx).Model(&model.Menu{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var menus []model.Menu
	result := query.Order("parent_id ASC, sort ASC, created_at ASC").Find(&menus)
	if result.Error != nil {
		return nil, result.Error
	}

	return buildMenuTree(menus, 0), nil
}

func buildMenuTree(menus []model.Menu, parentID uint) []model.Menu {
	var tree []model.Menu
	for i := range menus {
		if menus[i].ParentID == parentID {
			children := buildMenuTree(menus, menus[i].ID)
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

func (d *MenuDAO) CreateMenu(menu *model.Menu) error {
	return d.CreateMenuContext(context.Background(), menu)
}

func (d *MenuDAO) CreateMenuContext(ctx context.Context, menu *model.Menu) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Create(menu).Error
}

func (d *MenuDAO) UpdateMenu(menu *model.Menu) error {
	return d.UpdateMenuContext(context.Background(), menu)
}

func (d *MenuDAO) UpdateMenuContext(ctx context.Context, menu *model.Menu) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Save(menu).Error
}

func (d *MenuDAO) DeleteMenu(id uint) error {
	return d.DeleteMenuContext(context.Background(), id)
}

func (d *MenuDAO) DeleteMenuContext(ctx context.Context, id uint) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Menu{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("cannot delete menu with children")
		}

		if err := tx.Where("menu_id = ?", id).Delete(&model.MenuPermission{}).Error; err != nil {
			return err
		}

		return tx.Delete(&model.Menu{}, id).Error
	})
}

func (d *MenuDAO) AssignPermissions(menuID uint, permissionIDs []uint) error {
	return d.AssignPermissionsContext(context.Background(), menuID, permissionIDs)
}

func (d *MenuDAO) AssignPermissionsContext(ctx context.Context, menuID uint, permissionIDs []uint) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("menu_id = ?", menuID).Delete(&model.MenuPermission{}).Error; err != nil {
			return err
		}

		for _, permissionID := range permissionIDs {
			menuPermission := model.MenuPermission{
				MenuID:       menuID,
				PermissionID: permissionID,
			}
			if err := tx.Create(&menuPermission).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
