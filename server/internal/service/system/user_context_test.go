package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	authsvc "github.com/go-admin-kit/server/internal/service/auth"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserServiceCreateUserContextHonorsCanceledContext(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&UserService{}).CreateUserContext(ctx, CreateUserRequest{
		Username: "alice",
		Password: "Secret123",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateUserContext() error = %v, want context.Canceled", err)
	}
}

func TestUserServiceCreateUserContextReturnsUsernameLookupError(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnError(lookupErr)

	_, err := (&UserService{}).CreateUserContext(context.Background(), CreateUserRequest{
		Username: "alice",
		Password: "Secret123",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateUserContext() error = %v, want username lookup error", err)
	}
}

func TestUserServiceCreateUserContextRejectsWeakPassword(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password"}))

	_, err := (&UserService{}).CreateUserContext(context.Background(), CreateUserRequest{
		Username: "alice",
		Password: "short",
	})
	var validationErr authsvc.PasswordValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateUserContext() error = %T/%v, want PasswordValidationError", err, err)
	}
}

func setupSystemUserServiceContextTestDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})

	return mock
}
