package middleware

import (
	"context"
	"sync"

	"github.com/go-admin-kit/services/identity/internal/model"
)

// AuthUserStore loads users with roles for auth decisions.
type AuthUserStore interface {
	GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error)
}

// AuthPermissionStore loads permission codes for a user.
type AuthPermissionStore interface {
	GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error)
}

// ConsoleSessionValidator validates active console cookie sessions.
type ConsoleSessionValidator interface {
	ValidateActiveSessionContext(ctx context.Context, sessionID, username string) (*model.ConsoleSession, error)
}

// AuthMiddlewareDependencies groups the persistence used by auth middlewares.
type AuthMiddlewareDependencies struct {
	Users           AuthUserStore
	Permissions     AuthPermissionStore
	ConsoleSessions ConsoleSessionValidator
}

var (
	authDepsMu sync.RWMutex
	authDeps   AuthMiddlewareDependencies
)

// SetAuthMiddlewareDependencies installs the persistence behind the auth
// middlewares and returns a restore function. Middlewares read the current
// dependencies per request, so wiring only needs to happen before the first
// request is served.
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
