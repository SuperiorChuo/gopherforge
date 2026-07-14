package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	authsvc "github.com/go-admin-kit/server/internal/service/auth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserServiceCreateUserContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewUserServiceWithDB(db)
	_, err := svc.CreateUserContext(ctx, CreateUserRequest{
		Username: "alice",
		Password: "Secret123",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateUserContext() error = %v, want context.Canceled", err)
	}
}

func TestUserServiceCreateUserContextReturnsUsernameLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE username = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("alice", 1).
		WillReturnError(lookupErr)

	svc := NewUserServiceWithDB(db)
	_, err := svc.CreateUserContext(context.Background(), CreateUserRequest{
		Username: "alice",
		Password: "Secret123",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateUserContext() error = %v, want username lookup error", err)
	}
}

func TestUserServiceCreateUserContextRejectsWeakPassword(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE username = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password"}))

	svc := NewUserServiceWithDB(db)
	_, err := svc.CreateUserContext(context.Background(), CreateUserRequest{
		Username: "alice",
		Password: "short",
	})
	var validationErr authsvc.PasswordValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateUserContext() error = %T/%v, want PasswordValidationError", err, err)
	}
}

func setupSystemUserServiceContextTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
