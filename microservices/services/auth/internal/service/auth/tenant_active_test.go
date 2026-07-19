package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// A suspended tenant (status != 1) must be rejected so refresh is blocked.
func TestEnsureTenantActiveContextRejectsDisabledTenant(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(2, "acme", 0))

	svc := NewUserServiceWithDB(db)
	if err := svc.EnsureTenantActiveContext(context.Background(), 2); !errors.Is(err, ErrTenantDisabled) {
		t.Fatalf("EnsureTenantActiveContext() error = %v, want ErrTenantDisabled", err)
	}
}

// An enabled tenant passes.
func TestEnsureTenantActiveContextAllowsEnabledTenant(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(1, "default", 1))

	svc := NewUserServiceWithDB(db)
	if err := svc.EnsureTenantActiveContext(context.Background(), 1); err != nil {
		t.Fatalf("EnsureTenantActiveContext() error = %v, want nil", err)
	}
}

// Missing tenants row (pre-migration DB) is treated as active for compatibility.
func TestEnsureTenantActiveContextTreatsMissingTenantAsActive(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(9, 1).
		WillReturnError(errors.New("record not found"))

	svc := NewUserServiceWithDB(db)
	if err := svc.EnsureTenantActiveContext(context.Background(), 9); err != nil {
		t.Fatalf("EnsureTenantActiveContext() error = %v, want nil (compat)", err)
	}
}
