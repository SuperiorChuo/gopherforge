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

// 同一失效通道还要带走会话校验缓存：账号被禁用 / 删除时会走到这里，
// 而该缓存记的正是"会话有效且账号启用"，禁用后必须立刻不认。
func TestInvalidatePermissionCacheAlsoDropsConsoleSessionCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetConsoleSessionContext(ctx, "session-5", cache.ConsoleSessionIdentity{
		UserID:   5,
		Username: "alice",
	}); err != nil {
		t.Fatalf("seed console session cache: %v", err)
	}
	if _, ok := cacheService.GetConsoleSessionContext(ctx, "session-5"); !ok {
		t.Fatal("console session cache did not accept the seeded entry")
	}

	if err := InvalidatePermissionCacheForUsersContext(ctx, 5); err != nil {
		t.Fatalf("InvalidatePermissionCacheForUsersContext() error = %v", err)
	}

	if _, ok := cacheService.GetConsoleSessionContext(ctx, "session-5"); ok {
		t.Fatal("console session cache survived invalidation; a disabled account would stay logged in")
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

func TestInvalidatePermissionCacheAllAlsoDropsConsoleSessionCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetConsoleSessionContext(ctx, "session-11", cache.ConsoleSessionIdentity{
		UserID:   11,
		Username: "bob",
	}); err != nil {
		t.Fatalf("seed console session cache: %v", err)
	}

	if err := InvalidatePermissionCacheAllContext(ctx); err != nil {
		t.Fatalf("InvalidatePermissionCacheAllContext() error = %v", err)
	}

	if _, ok := cacheService.GetConsoleSessionContext(ctx, "session-11"); ok {
		t.Fatal("console session cache survived global invalidation")
	}
}

// 归一化权限头缓存（auth ForwardAuth verify）由 roles+permissions 推导，必须与
// 两者同漏斗失效：漏掉这里，网关会在 TTL（1h）内继续注入过期的 X-Auth-Permissions。
func TestInvalidatePermissionCacheAlsoDropsPermHeaderCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetUserPermHeaderContext(ctx, 5, "crm:read,crm:write"); err != nil {
		t.Fatalf("seed perm header cache: %v", err)
	}

	if err := InvalidatePermissionCacheForUsersContext(ctx, 5); err != nil {
		t.Fatalf("InvalidatePermissionCacheForUsersContext() error = %v", err)
	}

	if _, ok := cacheService.GetUserPermHeaderContext(ctx, 5); ok {
		t.Fatal("perm header cache survived invalidation; gateway would keep injecting revoked permissions")
	}
}

func TestInvalidatePermissionCacheAllAlsoDropsPermHeaderCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)

	ctx := context.Background()
	cacheService := cache.NewCacheService()
	if err := cacheService.SetUserPermHeaderContext(ctx, 7, "*"); err != nil {
		t.Fatalf("seed perm header cache: %v", err)
	}

	if err := InvalidatePermissionCacheAllContext(ctx); err != nil {
		t.Fatalf("InvalidatePermissionCacheAllContext() error = %v", err)
	}

	if _, ok := cacheService.GetUserPermHeaderContext(ctx, 7); ok {
		t.Fatal("perm header cache survived full invalidation")
	}
}
