package store

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// EnsureIndexes 的两条性质：①非 PG 方言下是 no-op（单测注入 sqlite 不能炸）；
// ②语句全部幂等——重复执行不报错，因为清一色 IF NOT EXISTS / IF EXISTS。
func TestEnsureIndexesSkipsNonPostgres(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s := NewWithDB(db)
	for i := 0; i < 3; i++ {
		if err := s.EnsureIndexes(); err != nil {
			t.Fatalf("run %d on sqlite: %v", i, err)
		}
	}
}

// 索引语句本身必须幂等且覆盖列表查询的排序列：跑不了 PG 的本机环境下，
// 用语句形状把约定钉住（漏了 IF NOT EXISTS 会让二次部署直接失败）。
func TestEnsureIndexStatementsAreIdempotent(t *testing.T) {
	for _, stmt := range ensureIndexStatements() {
		upper := strings.ToUpper(stmt)
		switch {
		case strings.HasPrefix(upper, "CREATE INDEX"), strings.HasPrefix(upper, "CREATE UNIQUE INDEX"):
			if !strings.Contains(upper, "IF NOT EXISTS") {
				t.Errorf("not idempotent: %s", stmt)
			}
		case strings.HasPrefix(upper, "DROP INDEX"):
			if !strings.Contains(upper, "IF EXISTS") {
				t.Errorf("not idempotent: %s", stmt)
			}
		default:
			t.Errorf("unexpected statement kind: %s", stmt)
		}
	}
}

// 列表查询是 WHERE tenant_id=? [AND status=?] ORDER BY id DESC，
// 必须有 (tenant_id, id DESC) 与 (tenant_id, status, id DESC) 两条索引。
func TestEnsureIndexesCoversListQueries(t *testing.T) {
	joined := strings.Join(ensureIndexStatements(), "\n")
	want := []string{
		"cc_calls (tenant_id, id DESC)",
		"cc_calls (tenant_id, status, id DESC)",
		"cc_call_charges (tenant_id, id DESC)",
		"cc_account_txns (tenant_id, id DESC)",
		"cc_qc_results (tenant_id, id DESC)",
		"cc_transcribe_jobs (status, id)",
		"cc_risk_list_entries (tenant_id, id DESC)",
		"cc_risk_attempts (tenant_id, created_at)",
		"cc_risk_attempts (tenant_id, callee, created_at)",
		"cc_risk_attempts (tenant_id, from_ext, created_at)",
		"cc_voice_notifies (tenant_id, id DESC)",
		"cc_voice_notifies (status, id)",
	}
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("missing index on %s", w)
		}
	}
}

func TestListWarnAccountsCapsAtHardLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s := NewWithDB(db)
	if err := db.AutoMigrate(&CCBillingAccount{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	accounts := make([]CCBillingAccount, 0, maxWarnAccounts+5)
	for i := 1; i <= maxWarnAccounts+5; i++ {
		accounts = append(accounts, CCBillingAccount{
			TenantID: uint64(i),
			Balance:  100,
			Status:   "normal",
		})
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("seed warn accounts: %v", err)
	}

	list, err := s.ListWarnAccounts(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListWarnAccounts: %v", err)
	}
	if len(list) != maxWarnAccounts {
		t.Fatalf("ListWarnAccounts len = %d, want hard cap %d", len(list), maxWarnAccounts)
	}
}
