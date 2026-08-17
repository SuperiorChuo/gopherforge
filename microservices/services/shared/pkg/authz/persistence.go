package authz

import (
	"context"
	"errors"
	"sync"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

// ErrPersistenceNotConfigured 表示在 SetPersistence 安装底层存储之前，
// 就调用了包级 authz 辅助函数。
var ErrPersistenceNotConfigured = errors.New("authz persistence is not configured")

// UserWithRolesStore 加载用户及其角色。
type UserWithRolesStore interface {
	GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error)
}

// UserPermissionsStore 加载用户的权限编码。
type UserPermissionsStore interface {
	GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error)
}

// Persistence 汇聚支撑包级 authz 辅助函数的各类存储。
type Persistence struct {
	Users       UserWithRolesStore
	Permissions UserPermissionsStore
	DataScope   DataScopeStore
}

var (
	persistenceMu sync.RWMutex
	persistence   Persistence
)

// SetPersistence 安装包级 authz 辅助函数背后的存储，并返回一个恢复函数。
// 辅助函数每次调用都会读取当前的 persistence，
// 因此只需在首个请求处理前完成装配即可。
func SetPersistence(p Persistence) func() {
	persistenceMu.Lock()
	previous := persistence
	persistence = p
	persistenceMu.Unlock()

	return func() {
		persistenceMu.Lock()
		persistence = previous
		persistenceMu.Unlock()
	}
}

func currentPersistence() Persistence {
	persistenceMu.RLock()
	defer persistenceMu.RUnlock()
	return persistence
}
