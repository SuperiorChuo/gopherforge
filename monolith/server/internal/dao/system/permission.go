package system

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

type PermissionManageDAO struct {
	db *gorm.DB
}

func NewPermissionManageDAO(db *gorm.DB) *PermissionManageDAO {
	return &PermissionManageDAO{db: db}
}

func (d *PermissionManageDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *PermissionManageDAO) GetPermissionByIDContext(ctx context.Context, id uint) (*model.Permission, error) {
	var permission model.Permission
	result := d.dbWithContext(ctx).First(&permission, id)
	return &permission, result.Error
}

func (d *PermissionManageDAO) GetPermissionByCodeContext(ctx context.Context, code string) (*model.Permission, error) {
	var permission model.Permission
	result := d.dbWithContext(ctx).Where("code = ?", code).First(&permission)
	return &permission, result.Error
}

func (d *PermissionManageDAO) GetPermissionListContext(ctx context.Context, req pagination.PageRequest, keyword string, permissionType *int8) ([]model.Permission, int64, error) {
	var permissions []model.Permission
	var total int64

	query := d.dbWithContext(ctx).Model(&model.Permission{})
	if keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR description LIKE ? OR path LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if permissionType != nil {
		query = query.Where("type = ?", *permissionType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("parent_id ASC, created_at ASC").
		Find(&permissions)

	return permissions, total, result.Error
}

func (d *PermissionManageDAO) GetPermissionTreeContext(ctx context.Context) ([]model.Permission, error) {
	var permissions []model.Permission
	result := d.dbWithContext(ctx).Order("parent_id ASC, created_at ASC").Find(&permissions)
	if result.Error != nil {
		return nil, result.Error
	}

	return buildPermissionTree(permissions, 0), nil
}

func buildPermissionTree(permissions []model.Permission, parentID uint) []model.Permission {
	var tree []model.Permission
	for i := range permissions {
		if permissions[i].ParentID == parentID {
			permissions[i].Children = buildPermissionTree(permissions, permissions[i].ID)
			tree = append(tree, permissions[i])
		}
	}
	return tree
}

func (d *PermissionManageDAO) CreatePermissionContext(ctx context.Context, permission *model.Permission) error {
	return d.dbWithContext(ctx).Create(permission).Error
}

func (d *PermissionManageDAO) UpdatePermissionContext(ctx context.Context, permission *model.Permission) error {
	return d.dbWithContext(ctx).Save(permission).Error
}

func (d *PermissionManageDAO) DeletePermissionContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Permission{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("cannot delete permission with children")
		}

		if err := tx.Where("permission_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Permission{}, id).Error
	})
}
