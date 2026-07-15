package system

import (
	"context"
	"errors"

	"github.com/go-admin-kit/services/system/internal/pkg/cache"
)

// InvalidatePermissionCacheAllContext drops every cached user permission set.
// Menu changes affect route visibility for all users, so a full flush is the
// simplest correct move. Mirrors the monolith's cache.go helper.
func InvalidatePermissionCacheAllContext(ctx context.Context) error {
	return cache.NewCacheService().DelAllUserPermissionsContext(ctx)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
