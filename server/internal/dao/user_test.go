package dao

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

func TestUserDAOGetUserByUsernameUsesSharedQuery(t *testing.T) {
	mock := setupDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(42, "alice"))

	user, err := (&UserDAO{}).GetUserByUsernameContext(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserByUsernameContext() error = %v", err)
	}
	if user.ID != 42 || user.Username != "alice" {
		t.Fatalf("user = %#v, want id=42 username=alice", user)
	}
}

func TestUserDAOGetUserWithRolesReturnsNotFound(t *testing.T) {
	mock := setupDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := (&UserDAO{}).GetUserWithRolesContext(context.Background(), 99)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetUserWithRolesContext() error = %v, want record not found", err)
	}
}

func TestUserDAOGetUserWithRolesContextHonorsCanceledContext(t *testing.T) {
	setupDAOTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&UserDAO{}).GetUserWithRolesContext(ctx, 99)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserWithRolesContext() error = %v, want context.Canceled", err)
	}
}

func TestUserDAOUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open injected sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet injected database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open injected gorm db: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE username = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(42, "alice"))

	user, err := NewUserDAO(db).GetUserByUsernameContext(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserByUsernameContext() error = %v", err)
	}
	if user.ID != 42 || user.Username != "alice" {
		t.Fatalf("user = %#v, want id=42 username=alice", user)
	}
}

func setupDAOTestDB(t *testing.T) sqlmock.Sqlmock {
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
