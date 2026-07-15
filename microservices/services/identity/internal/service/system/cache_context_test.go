package system

import (
	"context"
	"errors"
	"testing"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
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
