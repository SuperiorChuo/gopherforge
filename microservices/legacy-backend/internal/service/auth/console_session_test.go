package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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

func TestConsoleSessionServiceValidateActiveSessionContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewConsoleSessionServiceWithDB(db)
	_, err := svc.ValidateActiveSessionContext(ctx, "session-1", "alice")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateActiveSessionContext() error = %v, want context.Canceled", err)
	}
}

func TestConsoleSessionServiceValidateActiveSessionContextRejectsBlankSessionID(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)

	svc := NewConsoleSessionServiceWithDB(db)
	_, err := svc.ValidateActiveSessionContext(context.Background(), "   ", "alice")
	if !errors.Is(err, ErrConsoleSessionInvalid) {
		t.Fatalf("ValidateActiveSessionContext() error = %v, want ErrConsoleSessionInvalid", err)
	}
}
