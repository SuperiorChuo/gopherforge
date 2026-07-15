package system

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/identity/internal/model"
)

// PermissionCacheDAO provides relation lookups needed for permission cache invalidation.
type PermissionCacheDAO struct {
	db *gorm.DB
}

func NewPermissionCacheDAO(db *gorm.DB) *PermissionCacheDAO {
	return &PermissionCacheDAO{db: db}
}

func (d *PermissionCacheDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *PermissionCacheDAO) FindUserIDsByRoleIDsContext(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}

	var userIDs []uint
	err := d.dbWithContext(ctx).Model(&model.UserRole{}).
		Where("role_id IN ?", roleIDs).
		Distinct().
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (d *PermissionCacheDAO) FindRoleIDsByPermissionIDsContext(ctx context.Context, permissionIDs []uint) ([]uint, error) {
	if len(permissionIDs) == 0 {
		return nil, nil
	}

	var roleIDs []uint
	err := d.dbWithContext(ctx).Model(&model.RolePermission{}).
		Where("permission_id IN ?", permissionIDs).
		Distinct().
		Pluck("role_id", &roleIDs).Error
	return roleIDs, err
}
