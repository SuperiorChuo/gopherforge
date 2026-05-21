package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRoleDAOGetRoleByCodeUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedRBACDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `roles` WHERE code = ? ORDER BY `roles`.`id` LIMIT ?")).
		WithArgs("admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).AddRow(uint(7), "Admin", "admin"))

	role, err := NewRoleDAO(db).GetRoleByCode("admin")
	if err != nil {
		t.Fatalf("GetRoleByCode() error = %v", err)
	}
	if role.ID != 7 || role.Code != "admin" {
		t.Fatalf("role = %#v, want injected admin role", role)
	}
}

func TestRoleDAOGetRoleListContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := (&RoleDAO{}).GetRoleListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetRoleListContext() error = %v, want context.Canceled", err)
	}
}

func newInjectedRBACDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
