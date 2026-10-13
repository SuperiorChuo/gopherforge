package store

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRiskStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&CCRiskPolicy{}, &CCRiskListEntry{}, &CCRiskAttempt{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewWithDB(db)
}

func TestGetOrCreateAndSavePolicy(t *testing.T) {
	st := newRiskStore(t)
	ctx := context.Background()
	p, err := st.GetOrCreatePolicy(ctx, 2)
	if err != nil || p.TenantID != 2 || p.MaxRatePerMin != 0 {
		t.Fatalf("default policy: %+v %v", p, err)
	}
	p.OutboundStart = "09:00"
	p.OutboundEnd = "18:00"
	p.OutboundWeekdays = "1,2,3,4,5"
	p.MaxCallsPerNumberPerHour = 8
	p.BlockedPrefixes = "00"
	p.MaxRatePerMin = 150
	saved, err := st.SavePolicy(ctx, p)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.OutboundStart != "09:00" || saved.MaxCallsPerNumberPerHour != 8 || saved.MaxRatePerMin != 150 {
		t.Fatalf("saved = %+v", saved)
	}
	// 把上限清零必须写进去（Updates 走 map，不能丢 0）。
	saved.MaxCallsPerNumberPerHour = 0
	saved.MaxRatePerMin = 0
	cleared, err := st.SavePolicy(ctx, saved)
	if err != nil || cleared.MaxCallsPerNumberPerHour != 0 || cleared.MaxRatePerMin != 0 {
		t.Fatalf("clear zeros: %+v %v", cleared, err)
	}
}

func TestRiskListCRUDAndTenantIsolation(t *testing.T) {
	st := newRiskStore(t)
	ctx := context.Background()
	e, err := st.CreateRiskList(ctx, &CCRiskListEntry{TenantID: 2, Number: "13800138000", Match: "exact", ListType: "black"})
	if err != nil || e.ID == 0 {
		t.Fatalf("create: %+v %v", e, err)
	}
	if _, err := st.CreateRiskList(ctx, &CCRiskListEntry{TenantID: 2, Number: "13800138000", Match: "exact", ListType: "black"}); err == nil {
		t.Fatal("duplicate must fail")
	}
	list, err := st.ListRiskLists(ctx, 2)
	if err != nil || len(list) != 1 {
		t.Fatalf("list tenant2: %d %v", len(list), err)
	}
	other, err := st.ListRiskLists(ctx, 3)
	if err != nil || len(other) != 0 {
		t.Fatalf("tenant3 must be empty, got %d", len(other))
	}
	ok, err := st.DeleteRiskList(ctx, 3, e.ID)
	if err != nil || ok {
		t.Fatalf("cross-tenant delete: ok=%v err=%v", ok, err)
	}
	ok, err = st.DeleteRiskList(ctx, 2, e.ID)
	if err != nil || !ok {
		t.Fatalf("own delete: ok=%v err=%v", ok, err)
	}
}

func TestCountOutboundAttemptsWindows(t *testing.T) {
	st := newRiskStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	seed := func(ext, callee string, at time.Time) {
		t.Helper()
		if err := st.RecordRiskAttempt(ctx, 2, ext, callee, at); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed("1000", "13800001111", now.Add(-30*time.Minute))
	seed("1000", "13800001111", now.Add(-90*time.Minute)) // 超 1 小时，不计入被叫/分机小时窗
	seed("1001", "13900001111", now.Add(-10*time.Second))
	seed("1000", "13700001111", now.Add(-2*time.Minute)) // 超 1 分钟，不计入租户分钟窗

	c, err := st.CountOutboundAttempts(ctx, 2, "1000", "13800001111", now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if c.NumberLastHour != 1 {
		t.Fatalf("number last hour = %d, want 1", c.NumberLastHour)
	}
	if c.AgentLastHour != 2 { // 30min 138 + 2min 137
		t.Fatalf("agent last hour = %d, want 2", c.AgentLastHour)
	}
	if c.TenantLastMin != 1 { // 仅 10s 那条
		t.Fatalf("tenant last min = %d, want 1", c.TenantLastMin)
	}

	n, err := st.PurgeRiskAttempts(ctx, now.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
}
