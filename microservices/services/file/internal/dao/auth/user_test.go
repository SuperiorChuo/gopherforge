package auth

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserDAOGetUserByPhoneUsesInjectedDB(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE phone = $1 ORDER BY "users"."id" LIMIT $2`)).
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
	db, mock := newAuthDAOTestDB(t)
	now := time.Date(2026, 5, 22, 9, 10, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "totp_recovery_codes" WHERE user_id = $1`)).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("INSERT INTO \"totp_recovery_codes\"").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	mock.ExpectCommit()

	if err := NewUserDAO(db).ReplaceTOTPRecoveryCodesContext(context.Background(), 7, []string{"hash-a", "hash-b"}, now); err != nil {
		t.Fatalf("ReplaceTOTPRecoveryCodesContext() error = %v", err)
	}
}

func TestUserDAOListUnusedTOTPRecoveryCodesContext(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "totp_recovery_codes" WHERE user_id = $1 AND used_at IS NULL ORDER BY id ASC`)).
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
	db, mock := newAuthDAOTestDB(t)
	now := time.Date(2026, 5, 22, 9, 12, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"totp_recovery_codes\" SET .* WHERE user_id = \\$\\d+ AND id = \\$\\d+ AND used_at IS NULL").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), uint(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewUserDAO(db).MarkTOTPRecoveryCodeUsedContext(context.Background(), 7, 11, now); err != nil {
		t.Fatalf("MarkTOTPRecoveryCodeUsedContext() error = %v", err)
	}
}

// newAuthDAOTestDB returns a sqlmock-backed *gorm.DB for constructor
// injection into auth DAOs. It never touches the global database.DB.
func newAuthDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
