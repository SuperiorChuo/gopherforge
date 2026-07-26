package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRoleDAOGetRoleByCodeUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedRBACDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE code = $1 ORDER BY "roles"."id" LIMIT $2`)).
		WithArgs("admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).AddRow(uint(7), "Admin", "admin"))

	role, err := NewRoleDAO(db).GetRoleByCodeContext(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetRoleByCodeContext() error = %v", err)
	}
	if role.ID != 7 || role.Code != "admin" {
		t.Fatalf("role = %#v, want injected admin role", role)
	}
}

func TestRoleDAOGetRoleListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := NewRoleDAO(db).GetRoleListContext(ctx, pagination.PageRequest{Page: 1, PageSize: 10}, "")
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
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	return db, mock
}

func TestRoleDAOGetRoleListDoesNotPreloadPermissions(t *testing.T) {
	// sqlmock 严格断言：list 只允许 count、主查询、数据范围部门三条查询。
	// 若有人把 Preload("Permissions") 加回列表，多出的权限查询会因未预期而失败。
	db, mock := newInjectedRBACDAOTestDB(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "roles"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT \* FROM "roles"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).AddRow(uint(7), "Admin", "admin"))
	mock.ExpectQuery(`SELECT \* FROM "role_data_scope_departments"`).
		WillReturnRows(sqlmock.NewRows([]string{"role_id", "department_id"}))

	roles, total, err := NewRoleDAO(db).GetRoleListContext(context.Background(), pagination.PageRequest{Page: 1, PageSize: 10}, "")
	if err != nil {
		t.Fatalf("GetRoleListContext() error = %v", err)
	}
	if total != 1 || len(roles) != 1 {
		t.Fatalf("total = %d, len(roles) = %d, want 1/1", total, len(roles))
	}
	if len(roles[0].Permissions) != 0 {
		t.Fatalf("list preloaded %d permissions; list must not preload permissions", len(roles[0].Permissions))
	}
}
