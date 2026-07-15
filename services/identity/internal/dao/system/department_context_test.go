package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
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
	db, mock := newInjectedDepartmentMenuDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "departments" ORDER BY parent_id ASC, sort ASC, created_at ASC`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).AddRow(7, "Engineering", "eng"))

	depts, err := NewDepartmentDAO(db).GetAllContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetAllContext() error = %v", err)
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
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}
