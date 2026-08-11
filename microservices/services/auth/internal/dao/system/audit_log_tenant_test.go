package system

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	localmodel "github.com/go-admin-kit/services/auth/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuditLogCreateAndListUseTenantContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&localmodel.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
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

	ctx := tenant.WithContext(context.Background(), 42)
	result, err := dao.ListLogsContext(ctx, AuditLogListQuery{
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
