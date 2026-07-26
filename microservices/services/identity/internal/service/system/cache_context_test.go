package system

import (
	"context"
	"errors"
	"testing"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/pkg/cache"
)

func TestInvalidatePermissionCacheByRolesContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := InvalidatePermissionCacheByRolesContext(ctx, systemdao.NewPermissionCacheDAO(db), 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InvalidatePermissionCacheByRolesContext() error = %v, want context.Canceled", err)
	}
}

// 安全接线守卫：角色码缓存必须与权限缓存在同一处失效。漏掉这一步，被撤销
// super_admin 的用户仍能在 TTL（1h）内通过 PermissionMiddleware 的前置判定。
func TestInvalidatePermissionCacheAlsoDropsRoleCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetUserRolesContext(ctx, 5, []string{"super_admin"}); err != nil {
		t.Fatalf("seed role cache: %v", err)
	}
	if err := cacheService.SetUserPermissionsContext(ctx, 5, []string{"system:user:list"}); err != nil {
		t.Fatalf("seed permission cache: %v", err)
	}

	if err := InvalidatePermissionCacheForUsersContext(ctx, 5); err != nil {
		t.Fatalf("InvalidatePermissionCacheForUsersContext() error = %v", err)
	}

	roles, err := cacheService.GetUserRolesContext(ctx, 5)
	if err != nil {
		t.Fatalf("read role cache: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("role cache still holds %v after invalidation; revoked roles would stay effective", roles)
	}
}

func TestInvalidatePermissionCacheAllAlsoDropsRoleCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetUserRolesContext(ctx, 11, []string{"super_admin"}); err != nil {
		t.Fatalf("seed role cache: %v", err)
	}

	if err := InvalidatePermissionCacheAllContext(ctx); err != nil {
		t.Fatalf("InvalidatePermissionCacheAllContext() error = %v", err)
	}

	roles, err := cacheService.GetUserRolesContext(ctx, 11)
	if err != nil {
		t.Fatalf("read role cache: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("role cache still holds %v after global invalidation", roles)
	}
}
