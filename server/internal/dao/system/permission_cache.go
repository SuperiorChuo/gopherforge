package system

import (
	"context"

	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
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
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

// Deprecated: use FindUserIDsByRoleIDsContext instead.
func (d *PermissionCacheDAO) FindUserIDsByRoleIDs(roleIDs []uint) ([]uint, error) {
	return d.FindUserIDsByRoleIDsContext(context.Background(), roleIDs)
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

// Deprecated: use FindRoleIDsByPermissionIDsContext instead.
func (d *PermissionCacheDAO) FindRoleIDsByPermissionIDs(permissionIDs []uint) ([]uint, error) {
	return d.FindRoleIDsByPermissionIDsContext(context.Background(), permissionIDs)
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
