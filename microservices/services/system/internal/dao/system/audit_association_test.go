package system

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAssignMenuPermissionsWritesPlatformAuditRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.MenuPermission{}, &localmodel.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&model.MenuPermission{MenuID: 7, PermissionID: 1}).Error; err != nil {
		t.Fatal(err)
	}

	ctx := audittrail.WithTenantID(audittrail.WithActor(context.Background(), "operator", "admin-7"), 3)
	dao := NewMenuDAO(db)
	if err := dao.AssignPermissionsContext(ctx, 7, []uint{2, 3}); err != nil {
		t.Fatalf("AssignPermissionsContext() error = %v", err)
	}

	var logs []localmodel.AuditLog
	if err := db.Order("id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("load audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.TargetType != "menu_permissions" || log.TargetID != "7" || log.Action != "update" {
		t.Fatalf("target = %s/%s action=%s", log.TargetType, log.TargetID, log.Action)
	}
	// 菜单平台归属：FixedTenantID=1 忽略 ctx 租户 3
	if log.TenantID != 1 {
		t.Fatalf("tenant = %d, want 1 (platform menu)", log.TenantID)
	}
	if log.ActorID != "admin-7" {
		t.Fatalf("actor = %s", log.ActorID)
	}
	if before := log.BeforeJSON["permission_ids"]; before == nil {
		t.Fatalf("before missing permission_ids: %#v", log.BeforeJSON)
	}
	if after := log.AfterJSON["permission_ids"]; after == nil {
		t.Fatalf("after missing permission_ids: %#v", log.AfterJSON)
	}
}
