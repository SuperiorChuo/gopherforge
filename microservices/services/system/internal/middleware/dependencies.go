package middleware

import (
	"context"
	"sync"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

// AuthUserStore 为鉴权决策加载带角色的用户。
type AuthUserStore interface {
	GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error)
}

// AuthPermissionStore 加载用户的权限码。
type AuthPermissionStore interface {
	GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error)
}

// ConsoleSessionValidator 校验有效的控制台 cookie 会话。
type ConsoleSessionValidator interface {
	ValidateActiveSessionContext(ctx context.Context, sessionID, username string) (*model.ConsoleSession, error)
}

// AuthMiddlewareDependencies 汇总鉴权中间件使用的持久化组件。
type AuthMiddlewareDependencies struct {
	Users           AuthUserStore
	Permissions     AuthPermissionStore
	ConsoleSessions ConsoleSessionValidator
}

var (
	authDepsMu sync.RWMutex
	authDeps   AuthMiddlewareDependencies
)

// SetAuthMiddlewareDependencies 安装鉴权中间件背后的持久化组件，并返回一个
// 恢复函数。中间件在每次请求时读取当前依赖，因此装配只需在首个请求被处理前
// 完成即可。
func SetAuthMiddlewareDependencies(deps AuthMiddlewareDependencies) func() {
	authDepsMu.Lock()
	previous := authDeps
	authDeps = deps
	authDepsMu.Unlock()

	return func() {
		authDepsMu.Lock()
		authDeps = previous
		authDepsMu.Unlock()
	}
}

func currentAuthDeps() AuthMiddlewareDependencies {
	authDepsMu.RLock()
	defer authDepsMu.RUnlock()
	return authDeps
}
