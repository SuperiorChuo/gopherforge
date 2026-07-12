package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
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

func TestUserServiceRegisterContextReturnsUsernameLookupError(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
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

func setupAuthServiceContextTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

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

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}
