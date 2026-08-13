package audittrail

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/gorm"
)

type auditedOutboxProbe struct {
	ID       uint `gorm:"primaryKey"`
	TenantID uint
	Name     string
}

func (auditedOutboxProbe) TableName() string { return "audited_outbox_probes" }

func TestTransactionalOutboxInsertDoesNotReuseAuditedStatement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&auditedOutboxProbe{}, &outbox.Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	outbox.EnableTransactional()
	defer outbox.DisableTransactional()
	if err := Register(db, Config{Targets: []Target{{
		Table: "audited_outbox_probes", Model: &auditedOutboxProbe{}, TargetType: "probe",
		TenantField: "tenant_id", SnapshotFields: []string{"id", "tenant_id", "name"},
	}}}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx := WithActor(WithTenantID(t.Context(), 1), "user", "1")
	if err := db.WithContext(ctx).Create(&auditedOutboxProbe{TenantID: 1, Name: "probe"}).Error; err != nil {
		t.Fatalf("create audited row with transactional outbox: %v", err)
	}
	var events int64
	if err := db.Model(&outbox.Event{}).Count(&events).Error; err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if events != 1 {
		t.Fatalf("outbox events = %d, want 1", events)
	}
}
