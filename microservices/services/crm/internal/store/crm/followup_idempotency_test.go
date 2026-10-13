package store

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/idempotency"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openFollowUpTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:crm-followup-idem-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return st
}

func TestCreateFollowUpIdempotent(t *testing.T) {
	st := openFollowUpTestStore(t)
	ctx := context.Background()

	cu := &model.Customer{TenantID: 1, Name: "幂等客户", Phone: "13900000001"}
	if err := st.db.Create(cu).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	f1 := &model.FollowUp{
		TenantID: 1, TargetType: "customer", TargetID: cu.ID,
		Content: "挂机跟进", ByUserID: 7,
	}
	if err := st.CreateFollowUpIdempotent(ctx, f1, "cc-call-abc"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if f1.ID == 0 {
		t.Fatal("first followup id empty")
	}

	f2 := &model.FollowUp{
		TenantID: 1, TargetType: "customer", TargetID: cu.ID,
		Content: "挂机跟进", ByUserID: 7,
	}
	if err := st.CreateFollowUpIdempotent(ctx, f2, "cc-call-abc"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if f2.ID != f1.ID {
		t.Fatalf("retry id=%d want %d", f2.ID, f1.ID)
	}

	var n int64
	if err := st.db.Model(&model.FollowUp{}).Where("tenant_id = ? AND target_id = ?", 1, cu.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("followups = %d, want 1 (no double write)", n)
	}

	// 同键不同内容 → 冲突
	f3 := &model.FollowUp{
		TenantID: 1, TargetType: "customer", TargetID: cu.ID,
		Content: "不同内容", ByUserID: 7,
	}
	if err := st.CreateFollowUpIdempotent(ctx, f3, "cc-call-abc"); !errors.Is(err, idempotency.ErrKeyReuse) {
		t.Fatalf("want ErrKeyReuse, got %v", err)
	}
}
