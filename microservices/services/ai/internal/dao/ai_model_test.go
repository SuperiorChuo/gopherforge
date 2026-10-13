package dao

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// tenantCtx returns a context carrying the given tenant id (as the auth
// middleware does; see middleware/auth.go).
func tenantCtx(tid uint) context.Context {
	return context.WithValue(context.Background(), tenant.ContextKey, tid)
}

// platformAdminCtx returns a context flagged as platform admin.
func platformAdminCtx() context.Context {
	return context.WithValue(context.Background(), tenant.PlatformAdminContextKey, true)
}

// quotaFixture builds a minimal AIQuota for write-path tests.
func quotaFixture(scope string, scopeID *uint64) localmodel.AIQuota {
	return localmodel.AIQuota{Scope: scope, ScopeID: scopeID, Enabled: 1, ResetDayOfMonth: 1}
}

func TestAIUsageLogDAOListScopesToTenant(t *testing.T) {
	db, mock := newDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "ai_usage_logs" WHERE tenant_id = $1`) + "$").
		WithArgs(uint64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_usage_logs" WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(uint64(2), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id"}).AddRow(uint64(10), uint64(2)))

	logs, total, err := NewAIUsageLogDAO(db).List(tenantCtx(2), 1, 20, nil, nil, "", nil, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("List() total=%d len=%d, want 1/1", total, len(logs))
	}
}

func TestAIUsageLogDAOListPlatformAdminSeesAllTenants(t *testing.T) {
	db, mock := newDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "ai_usage_logs"`) + "$").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_usage_logs" ORDER BY created_at DESC LIMIT $1`)).
		WithArgs(20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uint64(1)))

	_, total, err := NewAIUsageLogDAO(db).List(platformAdminCtx(), 1, 20, nil, nil, "", nil, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("List() total = %d, want 3", total)
	}
}

func TestAIUsageLogDAOGetTopUsersScopesToTenant(t *testing.T) {
	db, mock := newDAOTestDB(t)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 30)

	mock.ExpectQuery(`(?s)SELECT user_id, username,.+FROM "ai_usage_logs" WHERE tenant_id = \$1.+GROUP BY user_id, username`).
		WithArgs(uint64(2), start, end, 10).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "total_requests", "total_tokens", "total_cost"}).
			AddRow(uint64(3), "alice", 5, 1000, 1.25))

	stats, err := NewAIUsageLogDAO(db).GetTopUsers(tenantCtx(2), start, end, 10)
	if err != nil {
		t.Fatalf("GetTopUsers() error = %v", err)
	}
	if len(stats) != 1 || stats[0].UserID != 3 || stats[0].Username != "alice" {
		t.Fatalf("GetTopUsers() = %+v, want alice(3)", stats)
	}
}

func TestAIUsageLogDAOGetUsageSummaryScopesToTenant(t *testing.T) {
	db, mock := newDAOTestDB(t)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\) as total_requests,.+FROM "ai_usage_logs" WHERE tenant_id = \$1`).
		WithArgs(uint64(2), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"total_requests", "success_requests", "total_tokens", "total_cost"}).
			AddRow(2, 2, 500, 0.5))

	summary, err := NewAIUsageLogDAO(db).GetUsageSummary(tenantCtx(2), start, end, nil, nil)
	if err != nil {
		t.Fatalf("GetUsageSummary() error = %v", err)
	}
	if summary.TotalRequests != 2 {
		t.Fatalf("GetUsageSummary() = %+v, want total_requests=2", summary)
	}
}

func TestAIQuotaDAODeleteRejectsCrossTenantQuota(t *testing.T) {
	db, mock := newDAOTestDB(t)

	// 记录属于租户 99，当前请求方是租户 2 → 拒绝，且不产生 DELETE。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_quotas" WHERE "ai_quotas"."id" = $1`)).
		WithArgs(uint64(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scope", "scope_id"}).AddRow(uint64(5), "tenant", uint64(99)))

	err := NewAIQuotaDAO(db).Delete(tenantCtx(2), 5)
	if !errors.Is(err, ErrQuotaScopeForbidden) {
		t.Fatalf("Delete() error = %v, want ErrQuotaScopeForbidden", err)
	}
}

func TestAIQuotaDAODeleteRejectsUserQuotaOfOtherTenant(t *testing.T) {
	db, mock := newDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_quotas" WHERE "ai_quotas"."id" = $1`)).
		WithArgs(uint64(6), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scope", "scope_id"}).AddRow(uint64(6), "user", uint64(77)))
	// 用户 77 不属于租户 2。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "users" WHERE id = $1 AND tenant_id = $2`)).
		WithArgs(uint64(77), uint64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := NewAIQuotaDAO(db).Delete(tenantCtx(2), 6)
	if !errors.Is(err, ErrQuotaScopeForbidden) {
		t.Fatalf("Delete() error = %v, want ErrQuotaScopeForbidden", err)
	}
}

func TestAIQuotaDAODeleteAllowsOwnTenantQuota(t *testing.T) {
	db, mock := newDAOTestDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_quotas" WHERE "ai_quotas"."id" = $1`)).
		WithArgs(uint64(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scope", "scope_id"}).AddRow(uint64(5), "tenant", uint64(2)))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "ai_quotas" WHERE "ai_quotas"."id" = $1`)).
		WithArgs(uint64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAIQuotaDAO(db).Delete(tenantCtx(2), 5); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestAIQuotaDAOUpsertRejectsGlobalScopeForTenantUser(t *testing.T) {
	db, _ := newDAOTestDB(t)

	q := quotaFixture("global", nil)
	err := NewAIQuotaDAO(db).Upsert(tenantCtx(2), &q)
	if !errors.Is(err, ErrQuotaScopeForbidden) {
		t.Fatalf("Upsert(global) error = %v, want ErrQuotaScopeForbidden", err)
	}
}

func TestAIQuotaDAOUpsertForcesTenantScopeIDToCurrentTenant(t *testing.T) {
	db, mock := newDAOTestDB(t)

	// 请求方是租户 2，恶意指定 scope_id=99 → 被强制改写为 2。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_quotas" WHERE scope = $1 AND scope_id = $2`)).
		WithArgs("tenant", uint64(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scope", "scope_id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_quotas"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uint64(11)))
	mock.ExpectCommit()

	evil := uint64(99)
	q := quotaFixture("tenant", &evil)
	if err := NewAIQuotaDAO(db).Upsert(tenantCtx(2), &q); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if q.ScopeID == nil || *q.ScopeID != 2 {
		t.Fatalf("Upsert() scope_id = %v, want forced to current tenant 2", q.ScopeID)
	}
}
