package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/pay/internal/model"
	"gorm.io/gorm"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	// 每测试独立内存库（cache=shared 会让全部测试共享同一实例、状态互漏）
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1) // 单连接串行，替代 PG 行锁语义
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func mustOrder(t *testing.T, s *Store) *model.Order {
	t.Helper()
	o, err := s.CreateOrder(context.Background(), 1, "demo_recharge", "biz-1", "测试充值", 990, 30*time.Minute)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return o
}

func TestCreateOrderIdempotentByBiz(t *testing.T) {
	s := testStore(t)
	o1 := mustOrder(t, s)
	o2, err := s.CreateOrder(context.Background(), 1, "demo_recharge", "biz-1", "重复建单", 990, 30*time.Minute)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if o1.OrderNo != o2.OrderNo {
		t.Fatalf("expected same order, got %s vs %s", o1.OrderNo, o2.OrderNo)
	}
	// closed 后同 biz 可重新开单
	if err := s.CloseOrder(context.Background(), 1, o1.OrderNo); err != nil {
		t.Fatalf("close: %v", err)
	}
	o3, err := s.CreateOrder(context.Background(), 1, "demo_recharge", "biz-1", "重开", 990, 30*time.Minute)
	if err != nil {
		t.Fatalf("recreate after close: %v", err)
	}
	if o3.OrderNo == o1.OrderNo {
		t.Fatal("expected new order after close")
	}
}

func TestPaidFlowAndIdempotentNotify(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.MarkPaying(context.Background(), 1, o.OrderNo, 1, "mock"); err != nil {
		t.Fatalf("mark paying: %v", err)
	}
	paid, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "TRADE1", 990)
	if err != nil {
		t.Fatalf("apply paid: %v", err)
	}
	if paid.Status != model.OrderPaid || paid.PaidAt == nil {
		t.Fatalf("bad paid state: %+v", paid)
	}
	// 重复回调 → ErrDuplicateApplied 且状态不变
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "TRADE1", 990); !errors.Is(err, ErrDuplicateApplied) {
		t.Fatalf("expected duplicate, got %v", err)
	}
	// 回调任务已排一条
	var jobs int64
	s.DB().Model(&model.CallbackJob{}).Where("order_id = ?", paid.ID).Count(&jobs)
	if jobs != 1 {
		t.Fatalf("expected 1 callback job, got %d", jobs)
	}
}

func TestAmountMismatchRejected(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "TRADE1", 991); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("expected amount mismatch, got %v", err)
	}
	got, _ := s.OrderByNo(1, o.OrderNo)
	if got.Status != model.OrderWaiting {
		t.Fatalf("order must stay waiting on mismatch, got %s", got.Status)
	}
}

func TestTerminalLocked(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "T", 990); err != nil {
		t.Fatalf("paid: %v", err)
	}
	if err := s.CloseOrder(context.Background(), 1, o.OrderNo); !errors.Is(err, ErrOrderTerminal) {
		t.Fatalf("close after paid must fail, got %v", err)
	}
	if _, err := s.MarkPaying(context.Background(), 1, o.OrderNo, 1, "mock"); !errors.Is(err, ErrOrderTerminal) {
		t.Fatalf("paying after paid must fail, got %v", err)
	}
}

func TestRefundAccumulationGuard(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "T", 990); err != nil {
		t.Fatalf("paid: %v", err)
	}
	r1, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 500, "部分退", 1)
	if err != nil {
		t.Fatalf("refund1: %v", err)
	}
	if _, err := s.ApplyRefundResult(context.Background(), 1, r1.RefundNo, "RF1", true); err != nil {
		t.Fatalf("apply refund1: %v", err)
	}
	// 二次退 490 = 恰好退满
	r2, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 490, "退尾款", 1)
	if err != nil {
		t.Fatalf("refund2: %v", err)
	}
	if _, err := s.ApplyRefundResult(context.Background(), 1, r2.RefundNo, "RF2", true); err != nil {
		t.Fatalf("apply refund2: %v", err)
	}
	// 超退 1 分被拒
	if _, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 1, "超退", 1); !errors.Is(err, ErrRefundExceeds) {
		t.Fatalf("expected exceeds, got %v", err)
	}
	got, _ := s.OrderByNo(1, o.OrderNo)
	if got.RefundedCents != 990 {
		t.Fatalf("refunded_cents = %d, want 990", got.RefundedCents)
	}
}

func TestRefundInflightGuard(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "T", 990); err != nil {
		t.Fatalf("paid: %v", err)
	}
	// 在途 600 未终态时再退 500 应被拒（600+500 > 990）
	if _, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 600, "在途", 1); err != nil {
		t.Fatalf("refund inflight: %v", err)
	}
	if _, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 500, "并发", 1); !errors.Is(err, ErrRefundExceeds) {
		t.Fatalf("expected exceeds with inflight, got %v", err)
	}
}

func TestRefundRequiresPaid(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.CreateRefund(context.Background(), 1, o.OrderNo, 100, "未支付退款", 1); !errors.Is(err, ErrOrderNotPaid) {
		t.Fatalf("expected not-paid, got %v", err)
	}
}

func TestExpireDueOrders(t *testing.T) {
	s := testStore(t)
	orders := make([]*model.Order, 3)
	for i, bizID := range []string{"exp-1", "exp-2", "exp-3"} {
		o, err := s.CreateOrder(context.Background(), 1, "demo_recharge", bizID, "立即过期", 100, -time.Minute)
		if err != nil {
			t.Fatalf("create %s: %v", bizID, err)
		}
		orders[i] = o
	}
	closed, err := s.ExpireDueOrders(2)
	if err != nil || closed != 2 {
		t.Fatalf("expire: closed=%d err=%v", closed, err)
	}
	for i, o := range orders {
		got, _ := s.OrderByNo(1, o.OrderNo)
		want := model.OrderClosed
		if i == 2 {
			want = model.OrderWaiting
		}
		if got.Status != want {
			t.Fatalf("order %d status=%s, want %s", i, got.Status, want)
		}
	}
	closed, err = s.ExpireDueOrders(2)
	if err != nil || closed != 1 {
		t.Fatalf("second expire: closed=%d err=%v", closed, err)
	}
}

func TestCallbackJobRetryToDead(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.ApplyPaid(context.Background(), 1, o.OrderNo, "T", 990); err != nil {
		t.Fatalf("paid: %v", err)
	}
	o2, err := s.CreateOrder(context.Background(), 1, "demo_recharge", "biz-2", "测试充值 2", 990, 30*time.Minute)
	if err != nil {
		t.Fatalf("create second order: %v", err)
	}
	if _, err := s.ApplyPaid(context.Background(), 1, o2.OrderNo, "T2", 990); err != nil {
		t.Fatalf("paid second order: %v", err)
	}
	jobs, err := s.ClaimCallbackJobs(10, time.Minute)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("claim: n=%d err=%v", len(jobs), err)
	}
	// 续租后再次 claim 应为空（NextAt 已推迟）
	again, _ := s.ClaimCallbackJobs(10, time.Minute)
	if len(again) != 0 {
		t.Fatalf("lease not applied, claimed %d", len(again))
	}
	// 重试到上限 → dead → 手工重推恢复 pending
	if err := s.RetryCallbackJob(jobs[0].ID, 8, 8, "boom"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	var job model.CallbackJob
	s.DB().First(&job, jobs[0].ID)
	if job.Status != "dead" {
		t.Fatalf("expected dead, got %s", job.Status)
	}
	if err := s.ReviveCallbackJob(1, job.ID); err != nil {
		t.Fatalf("revive: %v", err)
	}
	s.DB().First(&job, job.ID)
	if job.Status != "pending" || job.Attempts != 0 {
		t.Fatalf("revive failed: %+v", job)
	}
}

func TestListChannelsCapsAtHardLimit(t *testing.T) {
	s := testStore(t)
	for i := 0; i < maxChannelsPerTenant+3; i++ {
		ch := &model.Channel{TenantID: 1, Code: fmt.Sprintf("ch-%02d", i), Name: "渠道", Status: 1}
		if err := s.CreateChannel(context.Background(), ch); err != nil {
			t.Fatalf("create channel %d: %v", i, err)
		}
	}
	list, err := s.ListChannels(1)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(list) != maxChannelsPerTenant {
		t.Fatalf("ListChannels len = %d, want hard cap %d", len(list), maxChannelsPerTenant)
	}
}

func TestTenantIsolation(t *testing.T) {
	s := testStore(t)
	o := mustOrder(t, s)
	if _, err := s.OrderByNo(2, o.OrderNo); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("cross-tenant read must fail, got %v", err)
	}
	if err := s.CloseOrder(context.Background(), 2, o.OrderNo); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("cross-tenant close must fail, got %v", err)
	}
}
