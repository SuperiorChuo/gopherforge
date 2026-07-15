package system

import (
	"context"

	systemdao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/pkg/cache"
)

// PermissionCacheStore resolves the users affected by role or permission changes.
type PermissionCacheStore interface {
	FindUserIDsByRoleIDsContext(ctx context.Context, roleIDs []uint) ([]uint, error)
	FindRoleIDsByPermissionIDsContext(ctx context.Context, permissionIDs []uint) ([]uint, error)
}

func InvalidatePermissionCacheForUsersContext(ctx context.Context, userIDs ...uint) error {
	uniqueUserIDs := uniqueUint(userIDs)
	return cache.NewCacheService().DelUserPermissionsBatchContext(ctx, uniqueUserIDs)
}

func InvalidatePermissionCacheByRolesContext(ctx context.Context, store PermissionCacheStore, roleIDs ...uint) error {
	roleIDs = uniqueUint(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}

	if store == nil {
		store = &systemdao.PermissionCacheDAO{}
	}
	userIDs, err := store.FindUserIDsByRoleIDsContext(ctx, roleIDs)
	if err != nil {
		return err
	}

	return InvalidatePermissionCacheForUsersContext(ctx, userIDs...)
}

func InvalidatePermissionCacheByPermissionsContext(ctx context.Context, store PermissionCacheStore, permissionIDs ...uint) error {
	permissionIDs = uniqueUint(permissionIDs)
	if len(permissionIDs) == 0 {
		return nil
	}

	if store == nil {
		store = &systemdao.PermissionCacheDAO{}
	}
	roleIDs, err := store.FindRoleIDsByPermissionIDsContext(ctx, permissionIDs)
	if err != nil {
		return err
	}

	return InvalidatePermissionCacheByRolesContext(ctx, store, roleIDs...)
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
