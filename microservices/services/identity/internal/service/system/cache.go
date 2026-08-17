package system

import (
	"context"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/shared/pkg/cache"
)

// PermissionCacheStore resolves the users affected by role or permission changes.
type PermissionCacheStore interface {
	FindUserIDsByRoleIDsContext(ctx context.Context, roleIDs []uint) ([]uint, error)
	FindRoleIDsByPermissionIDsContext(ctx context.Context, permissionIDs []uint) ([]uint, error)
}

// Permission and role-code caches live and die together: every existing
// invalidation point (role changed / permission changed / user roles reassigned)
// routes through this function, so the role cache needs no new call sites —
// missing one would let a user whose super_admin was revoked keep passing the
// PermissionMiddleware probe until the TTL expires.
func InvalidatePermissionCacheForUsersContext(ctx context.Context, userIDs ...uint) error {
	uniqueUserIDs := uniqueUint(userIDs)
	cacheService := cache.NewCacheService()
	if err := cacheService.DelUserPermissionsBatchContext(ctx, uniqueUserIDs); err != nil {
		return err
	}
	if err := cacheService.DelUserRolesBatchContext(ctx, uniqueUserIDs); err != nil {
		return err
	}
	// The normalized permissions-header cache (auth ForwardAuth verify) is
	// derived from roles+permissions, so the three live and die together:
	// skipping this would let the gateway keep injecting a stale
	// X-Auth-Permissions header.
	if err := cacheService.DelUserPermHeaderBatchContext(ctx, uniqueUserIDs); err != nil {
		return err
	}
	// The console-session validation cache rides the same channel. It caches
	// "this session is live and this account is enabled", which a disable or a
	// delete falsifies — and both call this function.
	return cacheService.DelConsoleSessionsForUsersContext(ctx, uniqueUserIDs)
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
	cacheService := cache.NewCacheService()
	if err := cacheService.DelAllUserPermissionsContext(ctx); err != nil {
		return err
	}
	if err := cacheService.DelAllUserRolesContext(ctx); err != nil {
		return err
	}
	if err := cacheService.DelAllUserPermHeadersContext(ctx); err != nil {
		return err
	}
	return cacheService.DelAllConsoleSessionsContext(ctx)
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
