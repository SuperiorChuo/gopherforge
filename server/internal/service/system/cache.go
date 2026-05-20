package system

import (
	"context"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/pkg/cache"
)

// InvalidatePermissionCacheForUsers 清理指定用户的权限缓存。
func InvalidatePermissionCacheForUsers(userIDs ...uint) error {
	return InvalidatePermissionCacheForUsersContext(context.Background(), userIDs...)
}

func InvalidatePermissionCacheForUsersContext(ctx context.Context, userIDs ...uint) error {
	uniqueUserIDs := uniqueUint(userIDs)
	return cache.NewCacheService().DelUserPermissionsBatchContext(ctx, uniqueUserIDs)
}

// InvalidatePermissionCacheByRoles 清理拥有指定角色用户的权限缓存。
func InvalidatePermissionCacheByRoles(roleIDs ...uint) error {
	return InvalidatePermissionCacheByRolesContext(context.Background(), roleIDs...)
}

func InvalidatePermissionCacheByRolesContext(ctx context.Context, roleIDs ...uint) error {
	roleIDs = uniqueUint(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}

	userIDs, err := (&systemdao.PermissionCacheDAO{}).FindUserIDsByRoleIDsContext(ctx, roleIDs)
	if err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsersContext(ctx, userIDs...)
}

// InvalidatePermissionCacheByPermissions 清理拥有指定权限用户的权限缓存。
func InvalidatePermissionCacheByPermissions(permissionIDs ...uint) error {
	return InvalidatePermissionCacheByPermissionsContext(context.Background(), permissionIDs...)
}

func InvalidatePermissionCacheByPermissionsContext(ctx context.Context, permissionIDs ...uint) error {
	permissionIDs = uniqueUint(permissionIDs)
	if len(permissionIDs) == 0 {
		return nil
	}

	roleIDs, err := (&systemdao.PermissionCacheDAO{}).FindRoleIDsByPermissionIDsContext(ctx, permissionIDs)
	if err != nil {
		return err
	}

	return InvalidatePermissionCacheByRolesContext(ctx, roleIDs...)
}

// InvalidatePermissionCacheAll 清理全部用户权限缓存，用于影响面难以精准判断的权限资源变更。
func InvalidatePermissionCacheAll() error {
	return InvalidatePermissionCacheAllContext(context.Background())
}

func InvalidatePermissionCacheAllContext(ctx context.Context) error {
	return cache.NewCacheService().DelAllUserPermissionsContext(ctx)
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
