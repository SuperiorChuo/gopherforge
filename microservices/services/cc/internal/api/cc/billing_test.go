package api

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

func newBillingStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.CCRate{}, &store.CCBillingAccount{}, &store.CCCallCharge{}, &store.CCAccountTxn{}, &store.CCBill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.NewWithDB(db)
}

func TestMatchRateLongestPrefix(t *testing.T) {
	st := newBillingStore(t)
	ctx := context.Background()
	for _, r := range []store.CCRate{
		{TenantID: 1, Prefix: "0", Name: "国内长途", RatePerMin: 10, BillingStep: 60},
		{TenantID: 1, Prefix: "00", Name: "国际", RatePerMin: 200, BillingStep: 60},
		{TenantID: 1, Prefix: "1", Name: "手机", RatePerMin: 15, BillingStep: 60},
	} {
		if _, err := st.UpsertRate(ctx, &r); err != nil {
			t.Fatalf("seed rate %s: %v", r.Prefix, err)
		}
	}
	// "008610000" 应命中最长前缀 "00"（国际），不是 "0"。
	m, err := st.MatchRate(ctx, 1, "008610000")
	if err != nil || m == nil {
		t.Fatalf("match 008610000: %v %+v", err, m)
	}
	if m.Prefix != "00" {
		t.Fatalf("longest prefix = %q, want 00", m.Prefix)
	}
	// "075588888" 命中 "0"。
	m, _ = st.MatchRate(ctx, 1, "075588888")
	if m == nil || m.Prefix != "0" {
		t.Fatalf("match 0755 = %+v, want prefix 0", m)
	}
	// 未命中任何前缀 = nil（免费）。
	m, _ = st.MatchRate(ctx, 1, "9196")
	if m != nil {
		t.Fatalf("no-match should be nil, got %+v", m)
	}
}

func TestBillAmountStepRounding(t *testing.T) {
	rate := &store.CCRate{RatePerMin: 60, BillingStep: 60, ConnectFee: 10}
	// billsec=1 → 进位 60 秒 → 60*60/60 = 60 分 + 接通费 10 = 70。
	if got := billAmount(1, rate); got != 70 {
		t.Fatalf("billsec 1 = %d, want 70", got)
	}
	// billsec=61 → 进位 120 秒 → 120*60/60 = 120 + 10 = 130。
	if got := billAmount(61, rate); got != 130 {
		t.Fatalf("billsec 61 = %d, want 130", got)
	}
	// 6 秒步长：billsec=7 → 进位 12 秒 → 12*60/60 = 12 + 10 = 22。
	rate6 := &store.CCRate{RatePerMin: 60, BillingStep: 6, ConnectFee: 10}
	if got := billAmount(7, rate6); got != 22 {
		t.Fatalf("6s step billsec 7 = %d, want 22", got)
	}
}

func TestChargeCallDeductsAndIdempotent(t *testing.T) {
	st := newBillingStore(t)
	ctx := context.Background()
	if _, err := st.Recharge(ctx, 1, 10000, 1, "test"); err != nil { // 100 元
		t.Fatalf("recharge: %v", err)
	}
	charge := &store.CCCallCharge{TenantID: 1, CallID: "c-1", Callee: "075512345", BillSec: 60, Amount: 3000}
	got, err := st.ChargeCall(ctx, charge)
	if err != nil {
		t.Fatalf("charge: %v", err)
	}
	if got.BalanceAfter != 7000 {
		t.Fatalf("balance after = %d, want 7000", got.BalanceAfter)
	}
	// 重复入账（同 call_id）：余额不再扣，返回已有流水。
	again, err := st.ChargeCall(ctx, &store.CCCallCharge{TenantID: 1, CallID: "c-1", Amount: 3000})
	if err != nil {
		t.Fatalf("charge again: %v", err)
	}
	if again.BalanceAfter != 7000 {
		t.Fatalf("idempotent balance = %d, want 7000 (no double deduct)", again.BalanceAfter)
	}
	acc, _ := st.GetOrCreateAccount(ctx, 1)
	if acc.Balance != 7000 {
		t.Fatalf("account balance = %d, want 7000", acc.Balance)
	}
}

func TestSumChargesWindow(t *testing.T) {
	st := newBillingStore(t)
	ctx := context.Background()
	_, _ = st.Recharge(ctx, 1, 100000, 1, "test")
	_, _ = st.ChargeCall(ctx, &store.CCCallCharge{TenantID: 1, CallID: "s-1", Amount: 500, CreatedAt: time.Now()})
	_, _ = st.ChargeCall(ctx, &store.CCCallCharge{TenantID: 1, CallID: "s-2", Amount: 700, CreatedAt: time.Now()})
	sum, err := st.SumCharges(ctx, 1, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("sum: %v", err)
	}
	if sum != 1200 {
		t.Fatalf("sum = %d, want 1200", sum)
	}
}
