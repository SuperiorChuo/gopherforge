package system

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 关联表审计：批替换写路径在事务内落一行 audit_logs（before/after=关系 id 集合）。
func newAuditAssociationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserRole{}, &model.RolePermission{}, &model.RoleDataScopeDepartment{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func actorCtx(actorID string, tenantID uint) context.Context {
	return audittrail.WithTenantID(audittrail.WithActor(context.Background(), "operator", actorID), tenantID)
}

func loadAuditAssociation(t *testing.T, db *gorm.DB) []model.AuditLog {
	t.Helper()
	var logs []model.AuditLog
	if err := db.Order("id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("load audit logs: %v", err)
	}
	return logs
}

func TestAssignRolesContextWritesAuditRow(t *testing.T) {
	db := newAuditAssociationTestDB(t)
	if err := db.Create(&[]model.UserRole{{UserID: 5, RoleID: 1}, {UserID: 5, RoleID: 2}}).Error; err != nil {
		t.Fatal(err)
	}

	dao := NewUserDAO(db)
	if err := dao.AssignRolesContext(actorCtx("admin-7", 3), 5, []uint{3}); err != nil {
		t.Fatalf("AssignRolesContext() error = %v", err)
	}

	logs := loadAuditAssociation(t, db)
	if len(logs) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.TargetType != "user_roles" || log.TargetID != "5" || log.Action != "update" {
		t.Fatalf("target = %s/%s action=%s", log.TargetType, log.TargetID, log.Action)
	}
	if log.TenantID != 3 || log.ActorID != "admin-7" {
		t.Fatalf("tenant/actor = %d/%s", log.TenantID, log.ActorID)
	}
	if before := log.BeforeJSON["role_ids"]; before == nil {
		t.Fatalf("before missing role_ids: %#v", log.BeforeJSON)
	}
	if after := log.AfterJSON["role_ids"]; after == nil {
		t.Fatalf("after missing role_ids: %#v", log.AfterJSON)
	}
	// 关系本体已替换
	var cnt int64
	if err := db.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", 5, 3).Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("role 3 count = %d, want 1", cnt)
	}
}

func TestAssignRolesContextSkipsAuditWithoutActor(t *testing.T) {
	db := newAuditAssociationTestDB(t)
	dao := NewUserDAO(db)
	// 无 actor 的后台写：关系照常替换，审计静默跳过
	if err := dao.AssignRolesContext(context.Background(), 5, []uint{3}); err != nil {
		t.Fatalf("AssignRolesContext() error = %v", err)
	}
	if logs := loadAuditAssociation(t, db); len(logs) != 0 {
		t.Fatalf("audit rows = %d, want 0 (no actor)", len(logs))
	}
}

func TestAssignPermissionsContextWritesAuditRow(t *testing.T) {
	db := newAuditAssociationTestDB(t)
	if err := db.Create(&model.RolePermission{RoleID: 9, PermissionID: 10}).Error; err != nil {
		t.Fatal(err)
	}

	dao := NewRoleDAO(db)
	if err := dao.AssignPermissionsContext(actorCtx("admin-7", 3), 9, []uint{20, 30}); err != nil {
		t.Fatalf("AssignPermissionsContext() error = %v", err)
	}

	logs := loadAuditAssociation(t, db)
	if len(logs) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.TargetType != "role_permissions" || log.TargetID != "9" || log.TenantID != 3 {
		t.Fatalf("target = %s/%s tenant=%d", log.TargetType, log.TargetID, log.TenantID)
	}
	if before := log.BeforeJSON["permission_ids"]; before == nil {
		t.Fatalf("before missing permission_ids: %#v", log.BeforeJSON)
	}
	if after := log.AfterJSON["permission_ids"]; after == nil {
		t.Fatalf("after missing permission_ids: %#v", log.AfterJSON)
	}
}

func TestReplaceRoleDataScopeDepartmentsWritesAuditRow(t *testing.T) {
	db := newAuditAssociationTestDB(t)
	if err := db.Create(&model.RoleDataScopeDepartment{RoleID: 4, DepartmentID: 1}).Error; err != nil {
		t.Fatal(err)
	}

	ctx := actorCtx("admin-7", 3)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return replaceRoleDataScopeDepartments(ctx, tx, 4, "custom", []uint{2, 3})
	}); err != nil {
		t.Fatalf("replaceRoleDataScopeDepartments() error = %v", err)
	}

	logs := loadAuditAssociation(t, db)
	if len(logs) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.TargetType != "role_data_scope_departments" || log.TargetID != "4" {
		t.Fatalf("target = %s/%s", log.TargetType, log.TargetID)
	}
	if before := log.BeforeJSON["department_ids"]; before == nil {
		t.Fatalf("before missing department_ids: %#v", log.BeforeJSON)
	}
	// 非 custom 数据范围：删除后不插，after 应为空集
	if err := db.Transaction(func(tx *gorm.DB) error {
		return replaceRoleDataScopeDepartments(ctx, tx, 4, "self", nil)
	}); err != nil {
		t.Fatalf("replaceRoleDataScopeDepartments(self) error = %v", err)
	}
	logs = loadAuditAssociation(t, db)
	if len(logs) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(logs))
	}
	if after := logs[1].AfterJSON["department_ids"]; after == nil {
		t.Fatalf("self-scope after missing department_ids: %#v", logs[1].AfterJSON)
	}
}
