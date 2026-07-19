package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Disabling the default tenant (id=1) must be rejected before any UPDATE runs.
func TestTenantServiceUpdateRejectsDisablingDefaultTenant(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "status"}).
			AddRow(1, "default", "Default", 1))

	svc := NewTenantServiceWithDB(db)
	disabled := int8(0)
	_, err := svc.Update(context.Background(), 1, UpdateTenantRequest{Status: &disabled})
	if !errors.Is(err, ErrDefaultTenantLocked) {
		t.Fatalf("Update() error = %v, want ErrDefaultTenantLocked", err)
	}
}

// A non-default tenant can still be disabled.
func TestTenantServiceUpdateAllowsDisablingNonDefaultTenant(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "status"}).
			AddRow(2, "acme", "Acme", 1))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenants" SET`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewTenantServiceWithDB(db)
	disabled := int8(0)
	tt, err := svc.Update(context.Background(), 2, UpdateTenantRequest{Status: &disabled})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if tt.Status != 0 {
		t.Fatalf("Status = %d, want 0 (disabled)", tt.Status)
	}
}
