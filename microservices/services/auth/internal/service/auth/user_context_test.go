package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/auth/internal/config"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserServiceLoginPasswordContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewUserServiceWithDB(db)
	_, err := svc.LoginPasswordContext(ctx, "alice", "Password123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoginPasswordContext() error = %v, want context.Canceled", err)
	}
}

func TestUserServiceLoginPasswordAttributesAutomaticPasswordFlag(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	oldSecurity := config.Cfg.Security
	config.Cfg.Security.DefaultAdmin.ForceChangePassword = true
	config.Cfg.Security.DefaultAdmin.DefaultUsername = "admin"
	t.Cleanup(func() { config.Cfg.Security = oldSecurity })

	passwordHash := mustHashPasswordForTest(t, "admin123")
	mock.ExpectQuery(`SELECT \* FROM "tenants" WHERE code = \$1 AND status = 1`).
		WithArgs("acme", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "status"}).AddRow(7, "acme", 1))
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE tenant_id = \$1 AND username = \$2`).
		WithArgs(uint(7), "admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "username", "password", "status", "must_change_password",
		}).AddRow(1, 7, "admin", passwordHash, 1, false))
	mock.ExpectBegin()
	writeErr := errors.New("password flag write failed")
	mock.ExpectExec(`UPDATE "users" SET`).WillReturnError(writeErr)
	mock.ExpectRollback()

	var updateContext context.Context
	if err := db.Callback().Update().Before("gorm:update").Register("test:capture-login-audit-context", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "users" {
			updateContext = tx.Statement.Context
		}
	}); err != nil {
		t.Fatalf("register context capture callback: %v", err)
	}

	svc := NewUserServiceWithDB(db)
	_, err := svc.LoginPasswordWithTenantContext(context.Background(), "admin", "admin123", "acme", 0)
	if !errors.Is(err, writeErr) {
		t.Fatalf("LoginPasswordWithTenantContext() error = %v, want %v", err, writeErr)
	}
	actor, ok := sharedaudit.ActorFromContext(updateContext)
	if !ok || actor.Type != "user" || actor.ID != "admin" {
		t.Fatalf("password flag actor = %#v, found=%v", actor, ok)
	}
	if tenantID, ok := sharedaudit.TenantIDFromContext(updateContext); !ok || tenantID != 7 {
		t.Fatalf("password flag tenant = (%d, %v), want (7, true)", tenantID, ok)
	}
}

func TestUserServiceRegisterContextReturnsUsernameLookupError(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE tenant_id = $1 AND username = $2 ORDER BY "users"."id" LIMIT $3`)).
		WithArgs(uint(1), "alice", 1).
		WillReturnError(lookupErr)

	svc := NewUserServiceWithDB(db)
	_, err := svc.RegisterContext(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "Password123",
		Email:    "alice@example.com",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("RegisterContext() error = %v, want username lookup error", err)
	}
}

func TestUserServiceRegisterContextAttributesSelfRegistration(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE tenant_id = $1 AND username = $2 ORDER BY "users"."id" LIMIT $3`)).
		WithArgs(uint(1), "alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("alice@example.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectCommit()

	var createContext context.Context
	if err := db.Callback().Create().Before("gorm:create").Register("test:capture-registration-audit-context", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "users" {
			createContext = tx.Statement.Context
		}
	}); err != nil {
		t.Fatalf("register context capture callback: %v", err)
	}

	svc := NewUserServiceWithDB(db)
	user, err := svc.RegisterContext(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "Password123",
		Email:    "alice@example.com",
	})
	if err != nil {
		t.Fatalf("RegisterContext() error = %v", err)
	}
	if user.ID != 42 {
		t.Fatalf("registered user id = %d, want 42", user.ID)
	}
	actor, ok := sharedaudit.ActorFromContext(createContext)
	if !ok || actor.Type != "anonymous" || actor.ID != "self-registration" {
		t.Fatalf("registration actor = %#v, found=%v", actor, ok)
	}
	if tenantID, ok := sharedaudit.TenantIDFromContext(createContext); !ok || tenantID != 1 {
		t.Fatalf("registration tenant = (%d, %v), want (1, true)", tenantID, ok)
	}
}

func setupAuthServiceContextTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}
