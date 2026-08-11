package system

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuditLogCreateUsesTenantContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&localmodel.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}

	ctx := tenant.WithContext(context.Background(), 42)
	log := &localmodel.AuditLog{Action: "update", TargetType: "user", TargetID: "7"}
	if err := NewAuditLogDAO(db).CreateLogContext(ctx, log); err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	var tenantID uint
	if err := db.Raw(`SELECT tenant_id FROM audit_logs WHERE id = ?`, log.ID).Scan(&tenantID).Error; err != nil {
		t.Fatalf("read tenant: %v", err)
	}
	if tenantID != 42 {
		t.Fatalf("tenant_id = %d, want 42", tenantID)
	}
}

func TestAuditLogListIsTenantScoped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&localmodel.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	dao := NewAuditLogDAO(db)
	for _, tenantID := range []uint{42, 7} {
		ctx := tenant.WithContext(context.Background(), tenantID)
		if err := dao.CreateLogContext(ctx, &localmodel.AuditLog{
			Action: "update", TargetType: "user", TargetID: "7",
		}); err != nil {
			t.Fatalf("create tenant %d: %v", tenantID, err)
		}
	}

	result, err := dao.ListLogsContext(tenant.WithContext(context.Background(), 42), AuditLogListQuery{
		Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("list tenant 42: %v", err)
	}
	if result.Pagination.Total != 1 || len(result.Items) != 1 || result.Items[0].TenantID != 42 {
		t.Fatalf("tenant-scoped result: total=%d items=%+v", result.Pagination.Total, result.Items)
	}
	if result.Summary.TotalLogs != 1 {
		t.Fatalf("tenant-scoped summary total=%d", result.Summary.TotalLogs)
	}
}
