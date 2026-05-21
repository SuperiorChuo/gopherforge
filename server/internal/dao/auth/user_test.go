package auth

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserDAOGetUserByPhoneUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE phone = ? ORDER BY `users`.`id` LIMIT ?")).
		WithArgs("13800000000", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "phone"}).AddRow(42, "alice", "13800000000"))

	user, err := NewUserDAO(db).GetUserByPhone("13800000000")
	if err != nil {
		t.Fatalf("GetUserByPhone() error = %v", err)
	}
	if user.ID != 42 || user.Phone != "13800000000" {
		t.Fatalf("user = %#v, want id=42 phone=13800000000", user)
	}
}

func newInjectedAuthDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
