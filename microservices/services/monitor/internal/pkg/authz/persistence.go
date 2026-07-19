package authz

import (
	"context"
	"errors"
	"sync"

	"github.com/go-admin-kit/server/internal/model"
)

// ErrPersistenceNotConfigured reports that package-level authz helpers were
// used before SetPersistence installed their backing stores.
var ErrPersistenceNotConfigured = errors.New("authz persistence is not configured")

// UserWithRolesStore loads users with their roles.
type UserWithRolesStore interface {
	GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error)
}

// UserPermissionsStore loads permission codes for a user.
type UserPermissionsStore interface {
	GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error)
}

// Persistence groups the stores backing package-level authz helpers.
type Persistence struct {
	Users       UserWithRolesStore
	Permissions UserPermissionsStore
	DataScope   DataScopeStore
}

var (
	persistenceMu sync.RWMutex
	persistence   Persistence
)

// SetPersistence installs the stores behind package-level authz helpers and
// returns a restore function. Helpers read the current persistence per call,
// so wiring only needs to happen before the first request is served.
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
