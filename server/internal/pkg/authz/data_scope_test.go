package authz

import (
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestResolveUserDataScopeFallbacks(t *testing.T) {
	setupAuthzTestDB(t)

	tests := []struct {
		name          string
		user          *model.User
		wantScope     DataScope
		wantUserID    uint
		wantDeptID    uint
		wantDeptIDs   []uint
		wantRoleIDs   []uint
		wantRoleCodes []string
		wantAll       bool
	}{
		{
			name:      "nil user denies all",
			user:      nil,
			wantScope: DataScopeNone,
		},
		{
			name: "no roles falls back to self scope",
			user: &model.User{
				ID:           42,
				DepartmentID: 7,
			},
			wantScope:   DataScopeSelf,
			wantUserID:  42,
			wantDeptID:  7,
			wantDeptIDs: []uint{7},
		},
		{
			name: "unknown role keeps self scope and role metadata",
			user: &model.User{
				ID:           43,
				DepartmentID: 8,
				Roles: []model.Role{
					{ID: 3, Code: "auditor"},
				},
			},
			wantScope:     DataScopeSelf,
			wantUserID:    43,
			wantDeptID:    8,
			wantDeptIDs:   []uint{8},
			wantRoleIDs:   []uint{3},
			wantRoleCodes: []string{"auditor"},
		},
		{
			name: "configured department scope wins over self fallback",
			user: &model.User{
				ID:           44,
				DepartmentID: 9,
				Roles: []model.Role{
					{ID: 4, Code: "department_viewer", DataScope: string(DataScopeDepartment)},
				},
			},
			wantScope:     DataScopeDepartment,
			wantUserID:    44,
			wantDeptID:    9,
			wantDeptIDs:   []uint{9},
			wantRoleIDs:   []uint{4},
			wantRoleCodes: []string{"department_viewer"},
		},
		{
			name: "dept admin starts with own department as tree fallback",
			user: &model.User{
				ID:           45,
				DepartmentID: 10,
				Roles: []model.Role{
					{ID: 5, Code: "dept_admin"},
				},
			},
			wantScope:     DataScopeDepartmentTree,
			wantUserID:    45,
			wantDeptID:    10,
			wantDeptIDs:   []uint{10, 11, 12},
			wantRoleIDs:   []uint{5},
			wantRoleCodes: []string{"dept_admin"},
		},
		{
			name: "custom scope uses inline department ids before database fallback",
			user: &model.User{
				ID:           46,
				DepartmentID: 11,
				Roles: []model.Role{
					{
						ID:                     6,
						Code:                   "regional_admin",
						DataScope:              string(DataScopeCustom),
						DataScopeDepartmentIDs: []uint{15, 0, 12, 15},
						DataScopeDepartments: []model.RoleDataScopeDepartment{
							{DepartmentID: 14},
							{DepartmentID: 12},
						},
					},
				},
			},
			wantScope:     DataScopeCustom,
			wantUserID:    46,
			wantDeptID:    11,
			wantDeptIDs:   []uint{12, 14, 15},
			wantRoleIDs:   []uint{6},
			wantRoleCodes: []string{"regional_admin"},
		},
		{
			name: "admin can access all",
			user: &model.User{
				ID:           47,
				DepartmentID: 16,
				Roles: []model.Role{
					{ID: 7, Code: "admin"},
				},
			},
			wantScope:     DataScopeAll,
			wantUserID:    47,
			wantDeptID:    16,
			wantRoleIDs:   []uint{7},
			wantRoleCodes: []string{"admin"},
			wantAll:       true,
		},
		{
			name: "zero department does not invent department ids",
			user: &model.User{
				ID: 48,
			},
			wantScope:  DataScopeSelf,
			wantUserID: 48,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveUserDataScope(tt.user)

			if got.Scope != tt.wantScope {
				t.Fatalf("scope = %q, want %q", got.Scope, tt.wantScope)
			}
			if got.UserID != tt.wantUserID {
				t.Fatalf("user id = %d, want %d", got.UserID, tt.wantUserID)
			}
			if got.DepartmentID != tt.wantDeptID {
				t.Fatalf("department id = %d, want %d", got.DepartmentID, tt.wantDeptID)
			}
			if !slices.Equal(got.DepartmentIDs, tt.wantDeptIDs) {
				t.Fatalf("department ids = %#v, want %#v", got.DepartmentIDs, tt.wantDeptIDs)
			}
			if !slices.Equal(got.RoleIDs, tt.wantRoleIDs) {
				t.Fatalf("role ids = %#v, want %#v", got.RoleIDs, tt.wantRoleIDs)
			}
			if !slices.Equal(got.RoleCodes, tt.wantRoleCodes) {
				t.Fatalf("role codes = %#v, want %#v", got.RoleCodes, tt.wantRoleCodes)
			}
			if got.CanAccessAll() != tt.wantAll {
				t.Fatalf("can access all = %v, want %v", got.CanAccessAll(), tt.wantAll)
			}
		})
	}
}

func setupAuthzTestDB(t *testing.T) {
	t.Helper()

	resetDefaultDepartmentTreeCache()
	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}

	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11).
			AddRow(99, 98))

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		resetDefaultDepartmentTreeCache()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})
}
