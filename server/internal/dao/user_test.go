package dao

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserDAOGetUserByUsernameUsesSharedQuery(t *testing.T) {
	db, mock := newDAOTestDB(t)
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

func TestUserDAOGetUserWithRolesReturnsNotFound(t *testing.T) {
	db, mock := newDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE `users`.`id` = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := NewUserDAO(db).GetUserWithRolesContext(context.Background(), 99)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetUserWithRolesContext() error = %v, want record not found", err)
	}
}

func TestUserDAOGetUserWithRolesContextHonorsCanceledContext(t *testing.T) {
	db, _ := newDAOTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewUserDAO(db).GetUserWithRolesContext(ctx, 99)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserWithRolesContext() error = %v, want context.Canceled", err)
	}
}

func TestUserDAOUsesInjectedDB(t *testing.T) {
	db, mock := newDAOTestDB(t)

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

// newDAOTestDB returns a sqlmock-backed *gorm.DB for constructor injection.
// It never touches the global database.DB.
func newDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
