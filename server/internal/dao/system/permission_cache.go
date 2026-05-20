package system

import (
	"context"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

// PermissionCacheDAO provides relation lookups needed for permission cache invalidation.
type PermissionCacheDAO struct{}

func (d *PermissionCacheDAO) FindUserIDsByRoleIDs(roleIDs []uint) ([]uint, error) {
	return d.FindUserIDsByRoleIDsContext(context.Background(), roleIDs)
}

func (d *PermissionCacheDAO) FindUserIDsByRoleIDsContext(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}

	var userIDs []uint
	err := database.DB.WithContext(ctx).Model(&model.UserRole{}).
		Where("role_id IN ?", roleIDs).
		Distinct().
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (d *PermissionCacheDAO) FindRoleIDsByPermissionIDs(permissionIDs []uint) ([]uint, error) {
	return d.FindRoleIDsByPermissionIDsContext(context.Background(), permissionIDs)
}

func (d *PermissionCacheDAO) FindRoleIDsByPermissionIDsContext(ctx context.Context, permissionIDs []uint) ([]uint, error) {
	if len(permissionIDs) == 0 {
		return nil, nil
	}

	var roleIDs []uint
	err := database.DB.WithContext(ctx).Model(&model.RolePermission{}).
		Where("permission_id IN ?", permissionIDs).
		Distinct().
		Pluck("role_id", &roleIDs).Error
	return roleIDs, err
}
