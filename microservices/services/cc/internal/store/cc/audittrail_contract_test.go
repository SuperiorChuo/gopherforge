package store_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// TestCoreCCTablesMatchAuditTrailTargets guards the cc audit Target
// registration (main.go) against model drift: validateTargets fails at startup
// if a TableName/SnapshotFields/TenantField no longer matches the model, and
// this test catches it in CI instead of on the deploy box.
func TestCoreCCTablesMatchAuditTrailTargets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cc_audittrail_contract?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&store.Agent{}, &store.Contact{}, &store.CCRate{}, &store.CCBillingAccount{}, &store.CCQcKeyword{}, &store.CCRiskPolicy{}, &store.CCRiskListEntry{}, &auditLog{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	// Must mirror the Targets registered in cc/cmd/main.go.
	if err := sharedaudit.Register(db, sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &store.Agent{}, Table: "cc_agents", TargetType: "cc_agent", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "UserID", "Extension", "DisplayName"}},
		{Model: &store.Contact{}, Table: "cc_contacts", TargetType: "cc_contact", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Number", "Name", "Remark"}},
		{Model: &store.CCRate{}, Table: "cc_rates", TargetType: "cc_rate", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Prefix", "Name", "RatePerMin", "BillingStep", "ConnectFee"}},
		{Model: &store.CCBillingAccount{}, Table: "cc_billing_accounts", TargetType: "cc_billing_account", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Balance", "CreditLimit", "Status"}},
		{Model: &store.CCQcKeyword{}, Table: "cc_qc_keywords", TargetType: "cc_qc_keyword", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Word", "Type", "Remark"}},
		{Model: &store.CCRiskPolicy{}, Table: "cc_risk_policies", TargetType: "cc_risk_policy", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "OutboundStart", "OutboundEnd", "OutboundWeekdays", "MaxCallsPerNumberPerHour", "MaxCallsPerAgentPerHour", "MaxCallsPerTenantPerMin", "BlockedPrefixes", "MaxRatePerMin"}},
		{Model: &store.CCRiskListEntry{}, Table: "cc_risk_list_entries", TargetType: "cc_risk_list", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Number", "Match", "ListType", "Remark"}},
	}}); err != nil {
		t.Fatalf("Register() error = %v (Target drift)", err)
	}
}

type auditLog struct {
	ID         uint64         `gorm:"primaryKey"`
	TenantID   uint64         `gorm:"index"`
	ActorType  string         `gorm:"size:32"`
	ActorID    string         `gorm:"size:128"`
	Action     string         `gorm:"size:64"`
	TargetType string         `gorm:"size:64;index"`
	TargetID   string         `gorm:"size:128"`
	BeforeJSON map[string]any `gorm:"type:text;serializer:json"`
	AfterJSON  map[string]any `gorm:"type:text;serializer:json"`
	Summary    string         `gorm:"size:512"`
	CreatedAt  time.Time
}

func (auditLog) TableName() string { return "audit_logs" }
