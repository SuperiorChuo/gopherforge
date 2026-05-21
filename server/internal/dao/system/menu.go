package system

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type MenuDAO struct {
	db *gorm.DB
}

func NewMenuDAO(db *gorm.DB) *MenuDAO {
	return &MenuDAO{db: db}
}

func (d *MenuDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

var ErrMenuHasChildren = errors.New("cannot delete menu with children")

// Deprecated: use GetMenuByIDContext instead.
func (d *MenuDAO) GetMenuByID(id uint) (*model.Menu, error) {
	return d.GetMenuByIDContext(context.Background(), id)
}

func (d *MenuDAO) GetMenuByIDContext(ctx context.Context, id uint) (*model.Menu, error) {
	var menu model.Menu
	result := d.dbWithContext(ctx).First(&menu, id)
	return &menu, result.Error
}

// Deprecated: use GetMenuListContext instead.
func (d *MenuDAO) GetMenuList(req pagination.PageRequest, keyword string, status *int8) ([]model.Menu, int64, error) {
	return d.GetMenuListContext(context.Background(), req, keyword, status)
}

func (d *MenuDAO) GetMenuListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8) ([]model.Menu, int64, error) {
	var menus []model.Menu
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Menu{})
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

// Deprecated: use GetMenuTreeContext instead.
func (d *MenuDAO) GetMenuTree(status *int8) ([]model.Menu, error) {
	return d.GetMenuTreeContext(context.Background(), status)
}

func (d *MenuDAO) GetMenuTreeContext(ctx context.Context, status *int8) ([]model.Menu, error) {
	query := d.dbWithContext(ctx).Model(&model.Menu{})
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

// Deprecated: use CreateMenuContext instead.
func (d *MenuDAO) CreateMenu(menu *model.Menu) error {
	return d.CreateMenuContext(context.Background(), menu)
}

func (d *MenuDAO) CreateMenuContext(ctx context.Context, menu *model.Menu) error {
	return d.dbWithContext(ctx).Create(menu).Error
}

// Deprecated: use UpdateMenuContext instead.
func (d *MenuDAO) UpdateMenu(menu *model.Menu) error {
	return d.UpdateMenuContext(context.Background(), menu)
}

func (d *MenuDAO) UpdateMenuContext(ctx context.Context, menu *model.Menu) error {
	return d.dbWithContext(ctx).Save(menu).Error
}

// Deprecated: use DeleteMenuContext instead.
func (d *MenuDAO) DeleteMenu(id uint) error {
	return d.DeleteMenuContext(context.Background(), id)
}

func (d *MenuDAO) DeleteMenuContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Menu{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrMenuHasChildren
		}

		if err := tx.Where("menu_id = ?", id).Delete(&model.MenuPermission{}).Error; err != nil {
			return err
		}

		return tx.Delete(&model.Menu{}, id).Error
	})
}

// Deprecated: use AssignPermissionsContext instead.
func (d *MenuDAO) AssignPermissions(menuID uint, permissionIDs []uint) error {
	return d.AssignPermissionsContext(context.Background(), menuID, permissionIDs)
}

func (d *MenuDAO) AssignPermissionsContext(ctx context.Context, menuID uint, permissionIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
