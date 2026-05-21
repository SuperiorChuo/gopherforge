package authz

import (
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestApplyUserEntityScopeSQL(t *testing.T) {
	db := newDataScopeDryRunDB(t)

	tests := []struct {
		name     string
		scope    UserDataScope
		wantSQL  string
		wantVars []any
	}{
		{
			name:    "all scope leaves query unrestricted",
			scope:   UserDataScope{Scope: DataScopeAll},
			wantSQL: "SELECT * FROM `users`",
		},
		{
			name:     "department scope filters by department ids",
			scope:    UserDataScope{Scope: DataScopeDepartment, DepartmentIDs: []uint{10, 11}},
			wantSQL:  "SELECT * FROM `users` WHERE department_id IN (?,?)",
			wantVars: []any{uint(10), uint(11)},
		},
		{
			name:     "self scope filters by user id",
			scope:    UserDataScope{Scope: DataScopeSelf, UserID: 7},
			wantSQL:  "SELECT * FROM `users` WHERE id = ?",
			wantVars: []any{uint(7)},
		},
		{
			name:    "none scope denies rows",
			scope:   UserDataScope{Scope: DataScopeNone},
			wantSQL: "SELECT * FROM `users` WHERE 1 = 0",
		},
		{
			name:    "empty department scope denies rows",
			scope:   UserDataScope{Scope: DataScopeDepartment},
			wantSQL: "SELECT * FROM `users` WHERE 1 = 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var users []model.User
			stmt := ApplyUserEntityScope(db.Model(&model.User{}), tt.scope, "id", "department_id").Find(&users).Statement
			assertDataScopeSQL(t, stmt, tt.wantSQL, tt.wantVars)
		})
	}
}

func TestApplyOwnerScopeSQL(t *testing.T) {
	db := newDataScopeDryRunDB(t)

	tests := []struct {
		name     string
		scope    UserDataScope
		wantSQL  string
		wantVars []any
	}{
		{
			name:     "self scope filters by owner id",
			scope:    UserDataScope{Scope: DataScopeSelf, UserID: 7},
			wantSQL:  "SELECT * FROM `files` WHERE user_id = ?",
			wantVars: []any{uint(7)},
		},
		{
			name:     "department scope filters through user department subquery",
			scope:    UserDataScope{Scope: DataScopeDepartment, DepartmentIDs: []uint{20, 21}},
			wantSQL:  "SELECT * FROM `files` WHERE user_id IN (SELECT `id` FROM `users` WHERE department_id IN (?,?))",
			wantVars: []any{uint(20), uint(21)},
		},
		{
			name:    "none scope denies rows",
			scope:   UserDataScope{Scope: DataScopeNone},
			wantSQL: "SELECT * FROM `files` WHERE 1 = 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []model.File
			stmt := ApplyOwnerScope(db.Model(&model.File{}), tt.scope, "user_id").Find(&files).Statement
			assertDataScopeSQL(t, stmt, tt.wantSQL, tt.wantVars)
		})
	}
}

func newDataScopeDryRunDB(t *testing.T) *gorm.DB {
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
	return db
}

func assertDataScopeSQL(t *testing.T, stmt *gorm.Statement, wantSQL string, wantVars []any) {
	t.Helper()

	if gotSQL := stmt.SQL.String(); gotSQL != wantSQL {
		t.Fatalf("sql = %q, want %q", gotSQL, wantSQL)
	}
	if !slices.EqualFunc(stmt.Vars, wantVars, func(left, right any) bool { return left == right }) {
		t.Fatalf("vars = %#v, want %#v", stmt.Vars, wantVars)
	}
}
