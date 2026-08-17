package authz

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// init 注册测试用受管模型（原实现硬编码 fileModelType；注册表化后测试自行注册）。
func init() {
	RegisterScopedModel(reflect.TypeOf(model.File{}), ScopeByOwner)
}

func TestDataScopePluginNoDirectiveNoOps(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)

	var users []model.User
	stmt := db.Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"users\"", nil)
}

func TestDataScopePluginScopesUserQueries(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{10, 11},
	})

	var users []model.User
	stmt := db.WithContext(ctx).Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"users\" WHERE department_id IN ($1,$2)", []any{uint(10), uint(11)})
}

func TestDataScopePluginScopesOwnerQueries(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartmentTree,
		DepartmentIDs: []uint{20, 21},
	})

	var files []model.File
	stmt := db.WithContext(ctx).Model(&model.File{}).Find(&files).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"files\" WHERE user_id IN (SELECT id FROM users WHERE department_id IN ($1,$2))", []any{uint(20), uint(21)})
}

func TestDataScopePluginDisableDirectiveNoOps(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := DisableDataScope(EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{30, 31},
	}))

	var users []model.User
	stmt := db.WithContext(ctx).Model(&model.User{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"users\"", nil)
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

type localUser struct {
	ID           uint `gorm:"primaryKey"`
	DepartmentID uint
}

func (localUser) TableName() string { return "users" }

func TestDataScopePluginRegisteredUserEntity(t *testing.T) {
	RegisterScopedModel(reflect.TypeOf(localUser{}), ScopeByUserEntity)
	db := newDataScopePluginDryRunDB(t)
	ctx := EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{10, 11},
	})

	var users []localUser
	stmt := db.WithContext(ctx).Model(&localUser{}).Find(&users).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"users\" WHERE department_id IN ($1,$2)", []any{uint(10), uint(11)})
}

func TestDataScopePluginForceSelfScope(t *testing.T) {
	db := newDataScopePluginDryRunDB(t)
	ctx := ForceSelfScope(EnableDataScope(context.Background(), UserDataScope{
		Scope:         DataScopeDepartment,
		DepartmentIDs: []uint{40, 41},
	}), 7)

	var files []model.File
	stmt := db.WithContext(ctx).Model(&model.File{}).Find(&files).Statement

	assertDataScopeSQL(t, stmt, "SELECT * FROM \"files\" WHERE user_id = $1", []any{uint(7)})
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
	wantSQL := "SELECT * FROM \"files\" WHERE user_id IN (SELECT id FROM users WHERE department_id IN ($1,$2))"
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

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	if err := RegisterDataScopePlugin(db); err != nil {
		t.Fatalf("register plugin: %v", err)
	}
	return db
}
