package system

import (
	"context"

	"github.com/go-admin-kit/services/shared/pkg/cache"
)

// InvalidatePermissionCacheAllContext 删除所有缓存的用户权限集合。
// 菜单变更影响所有用户的路由可见性，因此整体清空是最简单且正确的做法。
// 与单体版 cache.go 的辅助函数保持一致。
func InvalidatePermissionCacheAllContext(ctx context.Context) error {
	return cache.NewCacheService().DelAllUserPermissionsContext(ctx)
}
