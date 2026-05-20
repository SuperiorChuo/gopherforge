package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserServiceLoginPasswordContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&UserService{}).LoginPasswordContext(ctx, "alice", "Password123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoginPasswordContext() error = %v, want context.Canceled", err)
	}
}

func TestUserServiceRegisterContextReturnsUsernameLookupError(t *testing.T) {
	mock := setupAuthServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnError(lookupErr)

	_, err := (&UserService{}).RegisterContext(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "Password123",
		Email:    "alice@example.com",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("RegisterContext() error = %v, want username lookup error", err)
	}
}

func setupAuthServiceContextTestDB(t *testing.T) sqlmock.Sqlmock {
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
