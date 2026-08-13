package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

func TestPermissionDiagnosticAllowsDirectRoleGrant(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectDiagnosticUser(mock, 7, 2, 1)
	expectDiagnosticRole(mock, 7, 4, "operator", "department")
	expectDiagnosticDataScopeDepartments(mock, 4)
	expectDiagnosticPermission(mock, 4, 21, "system:user:list")
	expectDiagnosticResource(mock, 21, "system:user:list")
	expectDiagnosticTenantWithoutPackage(mock, 2)

	svc := NewPermissionDiagnosticServiceWithDB(db)
	result, err := svc.DiagnoseContext(tenant.WithContext(context.Background(), 2), PermissionDiagnosticRequest{
		UserID: 7, Permission: "system:user:list",
	})
	if err != nil {
		t.Fatalf("DiagnoseContext() error = %v", err)
	}
	if !result.Allowed || result.MatchedBy != "permission:system:user:list" {
		t.Fatalf("result = %#v, want direct role grant", result)
	}
	if !result.Resource.Registered || result.Resource.Path != "/api/v1/users" || result.Resource.Method != "GET" {
		t.Fatalf("resource = %#v, want registered GET /api/v1/users", result.Resource)
	}
	if result.DataScope.Scope != "department" || len(result.DataScope.DepartmentIDs) != 1 || result.DataScope.DepartmentIDs[0] != 9 {
		t.Fatalf("data scope = %#v, want department 9", result.DataScope)
	}
}

func TestPermissionDiagnosticReportsPackageOverrun(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectDiagnosticUser(mock, 7, 2, 1)
	expectDiagnosticRole(mock, 7, 4, "operator", "self")
	expectDiagnosticDataScopeDepartments(mock, 4)
	expectDiagnosticPermission(mock, 4, 21, "system:user:list")
	expectDiagnosticResource(mock, 21, "system:user:list")
	expectDiagnosticTenantWithPackage(mock, 2, 5)
	expectDiagnosticPackage(mock, 5, `[]`)

	svc := NewPermissionDiagnosticServiceWithDB(db)
	result, err := svc.DiagnoseContext(tenant.WithContext(context.Background(), 2), PermissionDiagnosticRequest{
		UserID: 7, Permission: "system:user:list",
	})
	if err != nil {
		t.Fatalf("DiagnoseContext() error = %v", err)
	}
	if !result.Allowed || !result.Package.HasExistingOverrun || result.Package.AllowsPermission {
		t.Fatalf("package result = %#v, want existing overrun", result.Package)
	}
}

func TestPermissionDiagnosticReportsUnregisteredResource(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectDiagnosticUser(mock, 7, 2, 1)
	expectDiagnosticRole(mock, 7, 4, "operator", "self")
	expectDiagnosticDataScopeDepartments(mock, 4)
	expectDiagnosticPermission(mock, 4, 21, "system:user:list")
	expectMissingDiagnosticResource(mock, "not:registered")
	expectDiagnosticTenantWithoutPackage(mock, 2)

	svc := NewPermissionDiagnosticServiceWithDB(db)
	result, err := svc.DiagnoseContext(tenant.WithContext(context.Background(), 2), PermissionDiagnosticRequest{
		UserID: 7, Permission: "not:registered",
	})
	if err != nil {
		t.Fatalf("DiagnoseContext() error = %v", err)
	}
	if result.Resource.Registered || result.Allowed {
		t.Fatalf("result = %#v, want unregistered denied resource", result)
	}
}

func TestPermissionDiagnosticListsOptionsWithLimit(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(`SELECT \* FROM "permissions" WHERE code ILIKE \$1 OR name ILIKE \$2 OR path ILIKE \$3 ORDER BY code ASC LIMIT \$4`).
		WithArgs("%user%", "%user%", "%user%", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "path", "method"}).
			AddRow(21, "List users", "system:user:list", "/api/v1/users", "GET"))

	svc := NewPermissionDiagnosticServiceWithDB(db)
	options, err := svc.ListOptionsContext(context.Background(), " user ", 999)
	if err != nil {
		t.Fatalf("ListOptionsContext() error = %v", err)
	}
	if len(options) != 1 || options[0].Code != "system:user:list" {
		t.Fatalf("options = %#v", options)
	}
}

func TestPermissionDiagnosticHidesCrossTenantUser(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	expectDiagnosticUser(mock, 7, 2, 1)

	svc := NewPermissionDiagnosticServiceWithDB(db)
	_, err := svc.DiagnoseContext(tenant.WithContext(context.Background(), 3), PermissionDiagnosticRequest{
		UserID: 7, Permission: "system:user:list",
	})
	if !errors.Is(err, ErrPermissionDiagnosticUserNotFound) {
		t.Fatalf("DiagnoseContext() error = %v, want not found", err)
	}
}

func expectDiagnosticUser(mock sqlmock.Sqlmock, id, tenantID uint, status int8) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "username", "nickname", "department_id", "status"}).
			AddRow(id, tenantID, "alice", "Alice", 9, status))
}

func expectDiagnosticRole(mock sqlmock.Sqlmock, userID, roleID uint, code, dataScope string) {
	mock.ExpectQuery(`SELECT .* FROM "roles" JOIN user_roles ON user_roles.role_id = roles.id WHERE user_roles.user_id = \$1 ORDER BY roles.id ASC`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "code", "data_scope"}).
			AddRow(roleID, 2, "Operator", code, dataScope))
}

func expectDiagnosticPermission(mock sqlmock.Sqlmock, roleID, permissionID uint, code string) {
	mock.ExpectQuery(`SELECT rp.role_id, permissions\.\* FROM role_permissions rp JOIN permissions ON permissions.id = rp.permission_id WHERE rp.role_id IN \(\$1\) ORDER BY rp.role_id ASC, permissions.id ASC`).
		WithArgs(roleID).
		WillReturnRows(sqlmock.NewRows([]string{"role_id", "id", "name", "code", "type"}).AddRow(roleID, permissionID, "List users", code, 3))
}

func expectDiagnosticResource(mock sqlmock.Sqlmock, id uint, code string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "permissions" WHERE code = $1 ORDER BY "permissions"."id" LIMIT $2`)).
		WithArgs(code, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "description", "type", "path", "method"}).
			AddRow(id, "List users", code, "List users", 3, "/api/v1/users", "GET"))
}

func expectMissingDiagnosticResource(mock sqlmock.Sqlmock, code string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "permissions" WHERE code = $1 ORDER BY "permissions"."id" LIMIT $2`)).
		WithArgs(code, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}))
}

func expectDiagnosticDataScopeDepartments(mock sqlmock.Sqlmock, roleID uint) {
	mock.ExpectQuery(`SELECT .* FROM "role_data_scope_departments" WHERE "role_data_scope_departments"."role_id" = \$1`).
		WithArgs(roleID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "department_id"}))
}

func expectDiagnosticTenantWithoutPackage(mock sqlmock.Sqlmock, id uint) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "package_id"}).AddRow(id, nil))
}

func expectDiagnosticTenantWithPackage(mock sqlmock.Sqlmock, id, packageID uint) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1 ORDER BY "tenants"."id" LIMIT $2`)).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "package_id"}).AddRow(id, packageID))
}

func expectDiagnosticPackage(mock sqlmock.Sqlmock, id uint, permissionCodes string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_packages" WHERE "tenant_packages"."id" = $1 ORDER BY "tenant_packages"."id" LIMIT $2`)).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permission_codes", "status"}).AddRow(id, "Basic", permissionCodes, 1))
}
