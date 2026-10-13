package system

import (
	"context"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"testing"
	"time"
)

func TestCompleteSetGroupedAuditAndOperationStatistics(t *testing.T) {
	db := testdb.Open(t, &localmodel.AuditLog{}, &localmodel.OperationLog{})
	const n = fullread.BatchSize*2 + 1
	now := time.Now().UTC().Truncate(time.Second)
	audits := make([]localmodel.AuditLog, 0, n*2)
	operations := make([]localmodel.OperationLog, 0, n*2)
	for i := 0; i < n; i++ {
		for j := 0; j < 2; j++ {
			audits = append(audits, localmodel.AuditLog{Action: fmt.Sprintf("action-%04d", i), TargetType: "fixture", TargetID: "7", CreatedAt: now})
			operations = append(operations, localmodel.OperationLog{Module: fmt.Sprintf("module-%04d", i), Method: fmt.Sprintf("M%04d", i), Status: 200 + j*300, CreatedAt: now})
		}
	}
	for _, rows := range []any{&audits, &operations} {
		if err := db.CreateInBatches(rows, 128).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&localmodel.AuditLog{Action: "excluded", TargetType: "other", TargetID: "7"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&localmodel.OperationLog{Module: "excluded", Method: "OTHER", CreatedAt: now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	a := NewAuditLogDAO(db)
	q := db.Model(&localmodel.AuditLog{}).Where("target_type = ?", "fixture")
	testdb.Batches(t, db, func() {
		got, err := a.BuildSummary(q)
		if err != nil || got.TotalLogs != n*2 || got.DistinctActions != n || len(got.ActionBreakdown) != n {
			t.Fatalf("audit summary=%+v err=%v", got, err)
		}
		for i, row := range got.ActionBreakdown {
			if row.Action != fmt.Sprintf("action-%04d", i) || row.Count != 2 {
				t.Fatalf("group %d: %+v", i, row)
			}
		}
	})
	d := NewOperationLogDAO(db)
	from, to := now.Add(-time.Minute), now.Add(time.Minute)
	testdb.Batches(t, db, func() {
		got, err := d.GetLogStatsContext(context.Background(), &from, &to)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != n*2 || got.ErrorCount != n || len(got.ByModule) != n || len(got.ByMethod) != n {
			t.Fatalf("incomplete statistics: %+v", got)
		}
		for i := 0; i < n; i++ {
			if got.ByModule[fmt.Sprintf("module-%04d", i)] != 2 || got.ByMethod[fmt.Sprintf("M%04d", i)] != 2 {
				t.Fatalf("missing group %d", i)
			}
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := a.BuildSummary(q)
		if err == nil || got.ActionBreakdown != nil {
			t.Fatalf("partial groups=%d err=%v", len(got.ActionBreakdown), err)
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := d.GetLogStatsContext(context.Background(), &from, &to)
		if err == nil || got != nil {
			t.Fatalf("partial stats=%+v err=%v", got, err)
		}
	})
}
