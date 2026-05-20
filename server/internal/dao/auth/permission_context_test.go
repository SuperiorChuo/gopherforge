package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestPermissionDAOGetUserPermissionsContextHonorsCanceledContext(t *testing.T) {
	setupAuthPermissionContextTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PermissionDAO{}).GetUserPermissionsContext(ctx, 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserPermissionsContext() error = %v, want context.Canceled", err)
	}
}

func setupAuthPermissionContextTestDB(t *testing.T) sqlmock.Sqlmock {
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
