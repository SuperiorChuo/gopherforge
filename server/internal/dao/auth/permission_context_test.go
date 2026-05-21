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

func TestPermissionDAOGetUserPermissionsContextHonorsCanceledContext(t *testing.T) {
	setupAuthPermissionContextTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PermissionDAO{}).GetUserPermissionsContext(ctx, 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUserPermissionsContext() error = %v, want context.Canceled", err)
	}
}

func TestPermissionDAOUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT permissions.code FROM `users` JOIN user_roles ON users.id = user_roles.user_id JOIN roles ON user_roles.role_id = roles.id JOIN role_permissions ON roles.id = role_permissions.role_id JOIN permissions ON role_permissions.permission_id = permissions.id WHERE users.id = ?")).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("dashboard.view"))

	codes, err := NewPermissionDAO(db).GetUserPermissions(7)
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	if len(codes) != 1 || codes[0] != "dashboard.view" {
		t.Fatalf("codes = %v, want dashboard.view", codes)
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
