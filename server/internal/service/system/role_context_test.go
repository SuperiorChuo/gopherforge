package system

import (
	"context"
	"errors"
	"regexp"
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

func TestRoleServiceCreateRoleContextReturnsCodeLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `roles` WHERE code = ? ORDER BY `roles`.`id` LIMIT ?")).
		WithArgs("manager", 1).
		WillReturnError(lookupErr)

	_, err := (&RoleService{}).CreateRoleContext(context.Background(), CreateRoleRequest{
		Name: "Manager",
		Code: "manager",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateRoleContext() error = %v, want code lookup error", err)
	}
}
