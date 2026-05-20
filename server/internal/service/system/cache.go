package system

import (
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/cache"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

// InvalidatePermissionCacheForUsers 清理指定用户的权限缓存。
func InvalidatePermissionCacheForUsers(userIDs ...uint) error {
	uniqueUserIDs := uniqueUint(userIDs)
	return cache.NewCacheService().DelUserPermissionsBatch(uniqueUserIDs)
}

// InvalidatePermissionCacheByRoles 清理拥有指定角色用户的权限缓存。
func InvalidatePermissionCacheByRoles(roleIDs ...uint) error {
	roleIDs = uniqueUint(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}

	var userIDs []uint
	if err := database.DB.Model(&model.UserRole{}).
		Where("role_id IN ?", roleIDs).
		Distinct().
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsers(userIDs...)
}

// InvalidatePermissionCacheByPermissions 清理拥有指定权限用户的权限缓存。
func InvalidatePermissionCacheByPermissions(permissionIDs ...uint) error {
	permissionIDs = uniqueUint(permissionIDs)
	if len(permissionIDs) == 0 {
		return nil
	}

	var roleIDs []uint
	if err := database.DB.Model(&model.RolePermission{}).
		Where("permission_id IN ?", permissionIDs).
		Distinct().
		Pluck("role_id", &roleIDs).Error; err != nil {
		return err
	}

	return InvalidatePermissionCacheByRoles(roleIDs...)
}

// InvalidatePermissionCacheAll 清理全部用户权限缓存，用于影响面难以精准判断的权限资源变更。
func InvalidatePermissionCacheAll() error {
	return cache.NewCacheService().DelAllUserPermissions()
}

func uniqueUint(values []uint) []uint {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[uint]struct{}, len(values))
	unique := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
