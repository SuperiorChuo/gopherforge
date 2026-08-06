package model_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/identity/internal/model"
	tenantscope "github.com/go-admin-kit/services/shared/pkg/tenant"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	"gorm.io/gorm"
)

func TestCoreIdentityModelsMatchAuditTrailTargets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:identity_audittrail_contract?mode=memory&cache=shared"), &gorm.Config{
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

	if err := db.AutoMigrate(&model.User{}, &model.Role{}, &model.Department{}, &model.AuditLog{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := tenantscope.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	if err := sharedaudit.Register(db, sharedaudit.Config{Targets: []sharedaudit.Target{
		sharedaudit.UserTarget(&model.User{}),
		sharedaudit.RoleTarget(&model.Role{}),
		sharedaudit.DepartmentTarget(&model.Department{}),
	}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	ctx := sharedaudit.WithTenantID(sharedaudit.WithActor(context.Background(), "operator", "alice"), 7)
	user := model.User{
		TenantID: 99,
		Username: "audit-user",
		Password: "password-hash-must-not-leak",
		Email:    "alice@example.com",
		Phone:    "13812345678",
		Status:   1,
	}
	role := model.Role{TenantID: 99, Name: "Audit Role", Code: "audit-role", DataScope: "self"}
	department := model.Department{
		TenantID: 99,
		Name:     "Audit Department",
		Code:     "audit-department",
		Email:    "owner@example.com",
		Phone:    "13912345678",
		Status:   1,
	}
	creates := []struct {
		name  string
		value any
	}{
		{name: "user", value: &user},
		{name: "role", value: &role},
		{name: "department", value: &department},
	}
	for _, create := range creates {
		if err := db.WithContext(ctx).Create(create.value).Error; err != nil {
			t.Fatalf("Create(%s) error = %v", create.name, err)
		}
	}

	var logs []model.AuditLog
	if err := db.Order("id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("load audit logs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("audit log count = %d, want 3", len(logs))
	}
	wantTargets := []string{"user", "role", "department"}
	for i, log := range logs {
		if log.TenantID != 7 || log.ActorType != "operator" || log.ActorID != "alice" {
			t.Fatalf("log %d attribution = tenant:%d actor:%s/%s", i, log.TenantID, log.ActorType, log.ActorID)
		}
		if log.TargetType != wantTargets[i] || log.Action != "create" {
			t.Fatalf("log %d target = %s/%s, want create/%s", i, log.Action, log.TargetType, wantTargets[i])
		}
	}
	if logs[0].AfterJSON["password"] != mask.RedactedValue {
		t.Fatalf("user password = %#v, want redacted", logs[0].AfterJSON["password"])
	}
	if logs[0].AfterJSON["email"] != "a***e@example.com" || logs[0].AfterJSON["phone"] != "138****5678" {
		t.Fatalf("user personal fields = email:%#v phone:%#v", logs[0].AfterJSON["email"], logs[0].AfterJSON["phone"])
	}
	if logs[2].AfterJSON["email"] != "o***r@example.com" || logs[2].AfterJSON["phone"] != "139****5678" {
		t.Fatalf("department personal fields = email:%#v phone:%#v", logs[2].AfterJSON["email"], logs[2].AfterJSON["phone"])
	}
}
