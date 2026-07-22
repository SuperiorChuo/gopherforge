package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 套餐创建：名称查重未命中后插入成功。
func TestTenantPackageServiceCreate(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_packages" WHERE name = $1`)).
		WithArgs("基础版", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "tenant_packages"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	svc := NewTenantPackageServiceWithDB(db)
	p, err := svc.Create(context.Background(), CreateTenantPackageRequest{
		Name:            "基础版",
		PermissionCodes: []string{"system:user:list", "system:user:list", " ", "system:role:list"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	// 权限码去重去空白
	if len(p.PermissionCodes) != 2 {
		t.Fatalf("PermissionCodes = %v, want deduped 2 codes", p.PermissionCodes)
	}
	if p.Status != 1 {
		t.Fatalf("Status = %d, want default 1", p.Status)
	}
}

// 套餐名称必填。
func TestTenantPackageServiceCreateRequiresName(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)
	svc := NewTenantPackageServiceWithDB(db)
	_, err := svc.Create(context.Background(), CreateTenantPackageRequest{Name: "  "})
	if !errors.Is(err, ErrTenantPackageNameRequired) {
		t.Fatalf("Create() error = %v, want ErrTenantPackageNameRequired", err)
	}
}

// 删除守卫：有租户绑定时拒删，不应触发 DELETE。
func TestTenantPackageServiceDeleteRejectsWhenBound(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_packages" WHERE "tenant_packages"."id" = $1`)).
		WithArgs(3, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permission_codes", "status"}).
			AddRow(3, "基础版", `["system:user:list"]`, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "tenants" WHERE package_id = $1`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	svc := NewTenantPackageServiceWithDB(db)
	if err := svc.Delete(context.Background(), 3); !errors.Is(err, ErrTenantPackageInUse) {
		t.Fatalf("Delete() error = %v, want ErrTenantPackageInUse", err)
	}
}

// 越界拦截：租户绑定套餐后，分配套餐外权限码返回明确错误（携带越界码）。
func TestRoleServiceAssignPermissionsRejectsCodesOutsidePackage(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.MatchExpectationsInOrder(false)

	// 角色（tenant_id=2）+ 空预载
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE "roles"."id" = $1`)).
		WithArgs(5, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "code", "data_scope"}).
			AddRow(5, 2, "运营", "op", "self"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_data_scope_departments"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "department_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_permissions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "permission_id"}))
	// 租户绑定套餐 7
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1`)).
		WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "status", "package_id"}).
			AddRow(2, "acme", "Acme", 1, 7))
	// 套餐仅允许 system:user:list
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_packages" WHERE "tenant_packages"."id" = $1`)).
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permission_codes", "status"}).
			AddRow(7, "基础版", `["system:user:list"]`, 1))
	// 请求的权限 id 解析出的码含套餐外的 system:user:delete
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "code" FROM "permissions" WHERE id IN`)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).
			AddRow("system:user:list").AddRow("system:user:delete"))

	svc := NewRoleServiceWithDB(db)
	err := (&svc).AssignPermissionsContext(context.Background(), 5, AssignPermissionsRequest{
		PermissionIDs: []uint{11, 12},
	})
	var exceedErr *PermissionsExceedPackageError
	if !errors.As(err, &exceedErr) {
		t.Fatalf("AssignPermissionsContext() error = %v, want PermissionsExceedPackageError", err)
	}
	if len(exceedErr.Codes) != 1 || exceedErr.Codes[0] != "system:user:delete" {
		t.Fatalf("exceeded codes = %v, want [system:user:delete]", exceedErr.Codes)
	}
}

// 超管豁免：platform_admin 上下文标志绕过套餐校验，直接完成分配
// （sqlmock 未注册 tenants / tenant_packages 查询期望，若未豁免会因意外查询而失败）。
func TestRoleServiceAssignPermissionsAllowsPlatformAdmin(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE "roles"."id" = $1`)).
		WithArgs(5, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "code", "data_scope"}).
			AddRow(5, 2, "运营", "op", "self"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_data_scope_departments"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "department_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_permissions" WHERE`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "permission_id"}))
	// 分配事务：全删重建
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "role_permissions"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "role_permissions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	// 缓存失效：角色下无用户则不触发 Redis
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT "user_id" FROM "user_roles"`)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))

	ctx := context.WithValue(context.Background(), "platform_admin", true) //nolint:staticcheck // 与认证中间件的字符串键保持一致
	svc := NewRoleServiceWithDB(db)
	if err := (&svc).AssignPermissionsContext(ctx, 5, AssignPermissionsRequest{
		PermissionIDs: []uint{11},
	}); err != nil {
		t.Fatalf("AssignPermissionsContext() error = %v, want nil (platform admin exempt)", err)
	}
}

// 未绑定套餐的租户不受约束（package_id 为 NULL 时直接放行）。
func TestRoleServiceAssignPermissionsUnboundTenantUnrestricted(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE "roles"."id" = $1`)).
		WithArgs(5, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "code", "data_scope"}).
			AddRow(5, 2, "运营", "op", "self"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_data_scope_departments"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "department_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "role_permissions" WHERE`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "permission_id"}))
	// package_id 为 NULL → 不限
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants" WHERE "tenants"."id" = $1`)).
		WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "status", "package_id"}).
			AddRow(2, "acme", "Acme", 1, nil))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "role_permissions"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "role_permissions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT "user_id" FROM "user_roles"`)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))

	svc := NewRoleServiceWithDB(db)
	if err := (&svc).AssignPermissionsContext(context.Background(), 5, AssignPermissionsRequest{
		PermissionIDs: []uint{11},
	}); err != nil {
		t.Fatalf("AssignPermissionsContext() error = %v, want nil (unbound tenant)", err)
	}
}
