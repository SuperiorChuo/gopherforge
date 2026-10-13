package system

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openProvisionDB(t *testing.T) *gorm.DB {
	t.Helper()
	// AssignRoles 会刷权限缓存，需 miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	old := redisstore.Client
	rc := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	redisstore.Client = rc
	t.Cleanup(func() {
		_ = rc.Close()
		redisstore.Client = old
		mr.Close()
	})

	db, err := gorm.Open(sqlite.Open("file:tenant-provision-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&localmodel.Tenant{},
		&localmodel.User{},
		&localmodel.Department{},
		&localmodel.Post{},
		&localmodel.UserPost{},
		&model.Role{},
		&model.Permission{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.RoleDataScopeDepartment{},
		&model.TenantSetting{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 平台默认租户 id=1（删除守卫依赖）
	if err := db.Create(&localmodel.Tenant{ID: 1, Code: "default", Name: "Default", Status: 1, Plan: "enterprise"}).Error; err != nil {
		t.Fatalf("seed default tenant: %v", err)
	}
	// 种子权限：未绑套餐时开通管理员会拿到全量权限
	for _, code := range []string{"system:user:list", "system:role:list", "system:tenant:list"} {
		p := model.Permission{Name: code, Code: code, Type: 1}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("seed perm %s: %v", code, err)
		}
	}
	return db
}

// Create 必须返回初始管理员凭据，且库中有 admin 角色 + 用户 + 角色绑定 + 强制改密。
func TestTenantCreateProvisionsAdminRoleAndUser(t *testing.T) {
	db := openProvisionDB(t)
	// 租户插件：Create 使用 DisableScope；provision 用新租户 ctx
	_ = tenant.Register(db)

	svc := NewTenantServiceWithDB(db)
	tt, admin, err := svc.Create(context.Background(), CreateTenantRequest{
		Code: "acme-co",
		Name: "Acme Corp",
		Plan: "free",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tt == nil || tt.ID == 0 {
		t.Fatal("tenant id empty")
	}
	if admin == nil || admin.Username != "admin" || admin.InitialPassword == "" {
		t.Fatalf("admin creds = %+v", admin)
	}

	var user localmodel.User
	if err := db.Where("tenant_id = ? AND username = ?", tt.ID, "admin").First(&user).Error; err != nil {
		t.Fatalf("admin user: %v", err)
	}
	if !user.MustChangePassword {
		t.Fatal("provisioned admin must force password change")
	}

	var role model.Role
	if err := db.Where("tenant_id = ? AND code = ?", tt.ID, "admin").First(&role).Error; err != nil {
		t.Fatalf("admin role: %v", err)
	}
	var ur model.UserRole
	if err := db.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&ur).Error; err != nil {
		t.Fatalf("user_role: %v", err)
	}
	var rpCount int64
	if err := db.Model(&model.RolePermission{}).Where("role_id = ?", role.ID).Count(&rpCount).Error; err != nil {
		t.Fatal(err)
	}
	if rpCount < 3 {
		t.Fatalf("role permissions = %d, want >= 3 (all seeded)", rpCount)
	}
}

// 开通中途失败时，Delete 级联应清掉孤儿（用错误 code 复现不了；测 Delete 后无残留）。
func TestTenantDeleteCascadesAccountData(t *testing.T) {
	db := openProvisionDB(t)
	_ = tenant.Register(db)
	svc := NewTenantServiceWithDB(db)
	tt, _, err := svc.Create(context.Background(), CreateTenantRequest{Code: "gone", Name: "Gone"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(context.Background(), tt.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var n int64
	_ = db.Model(&localmodel.Tenant{}).Where("id = ?", tt.ID).Count(&n)
	if n != 0 {
		t.Fatalf("tenant rows left: %d", n)
	}
	_ = db.Model(&localmodel.User{}).Where("tenant_id = ?", tt.ID).Count(&n)
	if n != 0 {
		t.Fatalf("user rows left: %d", n)
	}
	_ = db.Model(&model.Role{}).Where("tenant_id = ?", tt.ID).Count(&n)
	if n != 0 {
		t.Fatalf("role rows left: %d", n)
	}
}
