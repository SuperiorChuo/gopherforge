package authz

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDataScopePluginNoDirectiveNoOps(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)

	var users []model.User
	stmt := db.Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM `users`", nil)
}

func TestDataScopePluginScopesUserQueries(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{10, 11},
	})

	var users []model.User
	stmt := db.WithContext(ctx).Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM `users` WHERE department_id IN (?,?)", []any{uint(10), uint(11)})
}

func TestDataScopePluginScopesOwnerQueries(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartmentTree,
		DepartmentIDs: []uint{20, 21},
	})

	var files []model.File
	stmt := db.WithContext(ctx).Model(&model.File{}).Find(&files).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))", []any{uint(20), uint(21)})
}

func TestDataScopePluginDisableDirectiveNoOps(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := DisableDataScope(EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{30, 31},
	}))

	var users []model.User
	stmt := db.WithContext(ctx).Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM `users`", nil)
}

func TestDataScopePluginSkipsAliasedTableQueries(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{30, 31},
	})

	var users []model.User
	stmt := db.WithContext(ctx).Table("users AS u").Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM users AS u", nil)
}

func TestDataScopePluginForceSelfScope(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := ForceSelfScope(EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{40, 41},
	}), 7)

	var files []model.File
	stmt := db.WithContext(ctx).Model(&model.File{}).Find(&files).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM `files` WHERE user_id = ?", []any{uint(7)})
}

func TestDataScopePluginOwnerScopeSubqueryDoesNotReenterPlugin(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeCustom,
		DepartmentIDs: []uint{50, 51},
	})

	var files []model.File
	stmt := db.WithContext(ctx).Model(&model.File{}).Find(&files).Statement

	gotSQL := stmt.SQL.String()
	wantSQL := "SELECT * FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))"
	if gotSQL != wantSQL {
		t.Fatalf("sql = %q, want %q", gotSQL, wantSQL)
	}
	if count := strings.Count(gotSQL, "department_id IN"); count != 1 {
		t.Fatalf("department scope clause count = %d, want 1", count)
	}
}

func newDataScopePluginDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	if err := RegisterDataScopePlugin(db); err != nil {
		t.Fatalf("register plugin: %v", err)
	}
	return db
}
