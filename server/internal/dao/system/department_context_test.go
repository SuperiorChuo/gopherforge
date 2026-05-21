package system

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

func TestDepartmentDAOGetTreeContextHonorsCanceledContext(t *testing.T) {
	db, _ := newInjectedDepartmentMenuDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewDepartmentDAO(db).GetTreeContext(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTreeContext() error = %v, want context.Canceled", err)
	}
}

func TestDepartmentDAOGetAllUsesInjectedDB(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `departments` ORDER BY parent_id ASC, sort ASC, created_at ASC")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).AddRow(7, "Engineering", "eng"))

	depts, err := NewDepartmentDAO(db).GetAll(nil)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(depts) != 1 || depts[0].Code != "eng" {
		t.Fatalf("GetAll() departments = %#v, want one injected row", depts)
	}
}

func newInjectedDepartmentMenuDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
