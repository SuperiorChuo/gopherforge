package auth

import (
	"context"
	"regexp"
	"testing"
	"time"

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

	user, err := NewUserDAO(db).GetUserByPhoneContext(context.Background(), "13800000000")
	if err != nil {
		t.Fatalf("GetUserByPhoneContext() error = %v", err)
	}
	if user.ID != 42 || user.Phone != "13800000000" {
		t.Fatalf("user = %#v, want id=42 phone=13800000000", user)
	}
}

func TestUserDAOReplaceTOTPRecoveryCodesContext(t *testing.T) {
	db, mock := newInjectedAuthDAOTestDB(t)
	now := time.Date(2026, 5, 22, 9, 10, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `totp_recovery_codes` WHERE user_id = ?")).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `totp_recovery_codes`").
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	if err := NewUserDAO(db).ReplaceTOTPRecoveryCodesContext(context.Background(), 7, []string{"hash-a", "hash-b"}, now); err != nil {
		t.Fatalf("ReplaceTOTPRecoveryCodesContext() error = %v", err)
	}
}

func TestUserDAOListUnusedTOTPRecoveryCodesContext(t *testing.T) {
	db, mock := newInjectedAuthDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `totp_recovery_codes` WHERE user_id = ? AND used_at IS NULL ORDER BY id ASC")).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "code_hash", "used_at"}).
			AddRow(uint(1), uint(7), "hash-a", nil).
			AddRow(uint(2), uint(7), "hash-b", nil))

	codes, err := NewUserDAO(db).ListUnusedTOTPRecoveryCodesContext(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListUnusedTOTPRecoveryCodesContext() error = %v", err)
	}
	if len(codes) != 2 || codes[0].ID != 1 || codes[1].CodeHash != "hash-b" {
		t.Fatalf("codes = %#v, want two unused codes", codes)
	}
}

func TestUserDAOMarkTOTPRecoveryCodeUsedContext(t *testing.T) {
	db, mock := newInjectedAuthDAOTestDB(t)
	now := time.Date(2026, 5, 22, 9, 12, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `totp_recovery_codes` SET .* WHERE user_id = \\? AND id = \\? AND used_at IS NULL").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), uint(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewUserDAO(db).MarkTOTPRecoveryCodeUsedContext(context.Background(), 7, 11, now); err != nil {
		t.Fatalf("MarkTOTPRecoveryCodeUsedContext() error = %v", err)
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
