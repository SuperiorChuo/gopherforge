package audittrail

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestRecordAssociationWritesRow(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	ctx := actorTenantContext("admin-7", 3)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return RecordAssociation(ctx, tx, RecordAssociationRequest{
			TargetType: "user_roles",
			TargetID:   "5",
			Action:     "update",
			Before:     map[string]any{"role_ids": []uint{1, 2}},
			After:      map[string]any{"role_ids": []uint{3}},
			Summary:    "update user 5 roles",
		})
	}); err != nil {
		t.Fatalf("RecordAssociation() error = %v", err)
	}

	log := onlyAuditLog(t, db)
	if log.TenantID != 3 || log.ActorType != "operator" || log.ActorID != "admin-7" {
		t.Fatalf("record tenant/actor = %d %s/%s", log.TenantID, log.ActorType, log.ActorID)
	}
	if log.TargetType != "user_roles" || log.TargetID != "5" || log.Action != "update" {
		t.Fatalf("record target = %s/%s action=%s", log.TargetType, log.TargetID, log.Action)
	}
	if got := log.BeforeJSON["role_ids"]; got == nil {
		t.Fatalf("before snapshot missing role_ids: %#v", log.BeforeJSON)
	}
	if log.Summary != "update user 5 roles" {
		t.Fatalf("summary = %q", log.Summary)
	}
}

func TestRecordAssociationSkipsWithoutActor(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	// 无 actor 的后台写静默跳过（与插件一致），不落行、不报错。
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RecordAssociation(context.Background(), tx, RecordAssociationRequest{
			TargetType: "user_roles", TargetID: "5", Action: "update",
		})
	}); err != nil {
		t.Fatalf("RecordAssociation() error = %v, want silent skip", err)
	}
	var count int64
	if err := db.Model(&storedAuditLog{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("audit rows = %d, want 0 (no actor)", count)
	}
}

func TestRecordAssociationRequiresTransaction(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	ctx := actorTenantContext("admin-7", 3)
	err := RecordAssociation(ctx, db, RecordAssociationRequest{
		TargetType: "user_roles", TargetID: "5", Action: "update",
	})
	if !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("RecordAssociation(non-tx) error = %v, want ErrTransactionRequired", err)
	}
}

func TestRecordAssociationRequiresTenant(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	ctx := WithActor(context.Background(), "operator", "admin-7") // 有 actor 无租户
	err := db.Transaction(func(tx *gorm.DB) error {
		return RecordAssociation(ctx, tx, RecordAssociationRequest{
			TargetType: "user_roles", TargetID: "5", Action: "update",
		})
	})
	if !errors.Is(err, ErrTenantContextRequired) {
		t.Fatalf("RecordAssociation(no tenant) error = %v, want ErrTenantContextRequired", err)
	}
}

func TestRecordAssociationFixedTenantOverridesContext(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	ctx := actorTenantContext("admin-7", 3)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RecordAssociation(ctx, tx, RecordAssociationRequest{
			TargetType:    "menu_permissions",
			TargetID:      "9",
			Action:        "update",
			FixedTenantID: 1, // 平台归属目标忽略 ctx 租户
		})
	}); err != nil {
		t.Fatalf("RecordAssociation() error = %v", err)
	}
	log := onlyAuditLog(t, db)
	if log.TenantID != 1 {
		t.Fatalf("record tenant = %d, want 1 (FixedTenantID)", log.TenantID)
	}
}

func TestRecordAssociationRollsBackWithTransaction(t *testing.T) {
	db := openBaseAuditTrailTestDB(t)
	ctx := actorTenantContext("admin-7", 3)
	// 审计行与业务写在同事务：回滚则两者都不落。
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAssociation(ctx, tx, RecordAssociationRequest{
			TargetType: "user_roles", TargetID: "5", Action: "update",
		}); err != nil {
			return err
		}
		return errors.New("business write failed")
	})
	if err == nil {
		t.Fatal("transaction should have failed")
	}
	var count int64
	if err := db.Model(&storedAuditLog{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("audit rows = %d, want 0 after rollback", count)
	}
}
