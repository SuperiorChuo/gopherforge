package audittrail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordActionPersistsSynchronouslyAndMasksSecrets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:audit_action?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&auditRecord{}); err != nil {
		t.Fatalf("migrate audit record: %v", err)
	}
	ctx := WithTenantID(WithActor(context.Background(), "operator", "admin"), 1)
	err = RecordAction(ctx, db, ActionRecord{
		Action: "export", TargetType: "edge_tls_certificate", TargetID: "7",
		Metadata: map[string]any{"domain": "admin.example.com", "private_key": "must-not-leak"},
	})
	if err != nil {
		t.Fatalf("RecordAction() error = %v", err)
	}
	var row auditRecord
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if row.Action != "export" || row.ActorID != "admin" || row.TargetID != "7" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if got := row.AfterJSON["private_key"]; got == "must-not-leak" || strings.Contains(row.Summary, "must-not-leak") {
		t.Fatalf("sensitive metadata leaked: after=%v summary=%q", row.AfterJSON, row.Summary)
	}
}

func TestRecordActionUsesTransactionalOutboxAndCommitsBeforeReturning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:audit_action_outbox?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&auditRecord{}, &outbox.Event{}); err != nil {
		t.Fatalf("migrate audit/outbox: %v", err)
	}
	outbox.EnableTransactional()
	t.Cleanup(outbox.DisableTransactional)

	ctx := WithTenantID(WithActor(context.Background(), "operator", "admin"), 1)
	if err := RecordAction(ctx, db, ActionRecord{
		Action: "export", TargetType: "edge_tls_certificate", TargetID: "7",
		Metadata: map[string]any{"domain": "admin.example.com", "private_key": "must-not-leak"},
	}); err != nil {
		t.Fatalf("RecordAction() error = %v", err)
	}
	var directRows int64
	if err := db.Model(&auditRecord{}).Count(&directRows).Error; err != nil || directRows != 0 {
		t.Fatalf("direct audit rows = %d, err=%v; want audit-service outbox ownership", directRows, err)
	}
	var queued outbox.Event
	if err := db.First(&queued).Error; err != nil {
		t.Fatalf("read outbox event: %v", err)
	}
	if queued.Subject != "audit.log.export" || queued.Status != outbox.StatusPending {
		t.Fatalf("unexpected outbox event: %+v", queued)
	}
	var event auditevents.AuditEvent
	if err := json.Unmarshal(queued.Payload, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	encoded := string(queued.Payload)
	if event.TargetID != "7" || strings.Contains(encoded, "must-not-leak") {
		t.Fatalf("unsafe/wrong event: %s", encoded)
	}
}

func TestRecordActionFailsClosedWithoutActor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:audit_action_no_actor?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := RecordAction(context.Background(), db, ActionRecord{
		TenantID: 1, Action: "export", TargetType: "edge_tls_certificate", TargetID: "7",
	}); err == nil {
		t.Fatal("RecordAction() succeeded without an explicit actor")
	}
}
