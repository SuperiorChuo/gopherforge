package system

import (
	"context"
	"errors"
	"testing"
)

func TestRoleServiceCreateRoleContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&RoleService{}).CreateRoleContext(ctx, CreateRoleRequest{
		Name: "Manager",
		Code: "manager",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateRoleContext() error = %v, want context.Canceled", err)
	}
}
