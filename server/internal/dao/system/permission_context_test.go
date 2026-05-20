package system

import (
	"context"
	"errors"
	"testing"
)

func TestPermissionManageDAOGetPermissionTreeContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PermissionManageDAO{}).GetPermissionTreeContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetPermissionTreeContext() error = %v, want context.Canceled", err)
	}
}
