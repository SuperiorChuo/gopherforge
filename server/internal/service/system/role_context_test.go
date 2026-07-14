package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

func TestRoleServiceCreateRoleContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewRoleServiceWithDB(db)
	_, err := (&svc).CreateRoleContext(ctx, CreateRoleRequest{
		Name: "Manager",
		Code: "manager",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateRoleContext() error = %v, want context.Canceled", err)
	}
}

func TestRoleServiceCreateRoleContextReturnsCodeLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE code = $1 ORDER BY "roles"."id" LIMIT $2`)).
		WithArgs("manager", 1).
		WillReturnError(lookupErr)

	svc := NewRoleServiceWithDB(db)
	_, err := (&svc).CreateRoleContext(context.Background(), CreateRoleRequest{
		Name: "Manager",
		Code: "manager",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateRoleContext() error = %v, want code lookup error", err)
	}
}
