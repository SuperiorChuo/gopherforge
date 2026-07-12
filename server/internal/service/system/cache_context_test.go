package system

import (
	"context"
	"errors"
	"testing"
)

func TestInvalidatePermissionCacheByRolesContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := InvalidatePermissionCacheByRolesContext(ctx, nil, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InvalidatePermissionCacheByRolesContext() error = %v, want context.Canceled", err)
	}
}
