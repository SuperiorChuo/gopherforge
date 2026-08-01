package model_test

import (
	"context"
	"testing"

	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/system/internal/model"
	tenantscope "github.com/go-admin-kit/services/system/internal/pkg/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMenuModelMatchesFixedTenantAuditTrailTarget(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:system_audittrail_contract?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.Menu{}, &model.AuditLog{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := tenantscope.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	if err := sharedaudit.Register(db, sharedaudit.Config{
		Targets: []sharedaudit.Target{sharedaudit.MenuTarget(&model.Menu{})},
	}); err != nil {
		t.Fatalf("register audit plugin: %v", err)
	}

	ctx := sharedaudit.WithTenantID(sharedaudit.WithActor(context.Background(), "operator", "tenant-admin"), 42)
	menu := model.Menu{Name: "audit-menu", Title: "Audit Menu", Path: "/audit", Status: 1}
	if err := db.WithContext(ctx).Create(&menu).Error; err != nil {
		t.Fatalf("Create(menu) error = %v", err)
	}

	var log model.AuditLog
	if err := db.First(&log).Error; err != nil {
		t.Fatalf("load audit log: %v", err)
	}
	if log.TenantID != 1 {
		t.Fatalf("menu audit tenant = %d, want platform tenant 1", log.TenantID)
	}
	if log.ActorType != "operator" || log.ActorID != "tenant-admin" {
		t.Fatalf("menu audit actor = %s/%s", log.ActorType, log.ActorID)
	}
	if log.Action != "create" || log.TargetType != "menu" || log.TargetID == "" {
		t.Fatalf("menu audit target = %s %s/%s", log.Action, log.TargetType, log.TargetID)
	}
}
