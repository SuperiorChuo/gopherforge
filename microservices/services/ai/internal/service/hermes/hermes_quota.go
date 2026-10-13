package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// ErrHermesQuotaExceeded 表示调用被额度拦下。与 ErrHermesProxyForbidden 分开，
// 因为语义不同：前者是用超了（稍后或次日可恢复），后者是没有权限。
var ErrHermesQuotaExceeded = errors.New("hermes model quota exceeded")

// HermesQuotaGuard 在模型代理转发前校验日/月额度。
//
// 判定口径与 middleware.AIQuotaChecker 保持一致（同一套 AIQuota 配置、同一个
// GetUsageSummary 统计），区别只在于这里不依赖 gin 上下文——代理走 bearer token
// 鉴权，不经过 AuthMiddleware，拿不到 gin 里的 user_id。
type HermesQuotaGuard struct {
	quotas *AIQuotaService
	usage  *dao.AIUsageLogDAO
	now    func() time.Time
}

func NewHermesQuotaGuard(quotas *AIQuotaService, usage *dao.AIUsageLogDAO, now func() time.Time) *HermesQuotaGuard {
	if now == nil {
		now = time.Now
	}
	return &HermesQuotaGuard{quotas: quotas, usage: usage, now: now}
}

// Check 在超限时返回包装 ErrHermesQuotaExceeded 的错误。
//
// 统计查询失败时放行（与既有中间件同口径）：额度是成本控制手段，不该因为一次
// 统计查询抖动就切断员工的正常使用。
func (g *HermesQuotaGuard) Check(ctx context.Context, userID, tenantID uint) error {
	if g == nil || g.quotas == nil || g.usage == nil {
		return nil
	}
	uid := uint64(userID)
	var tid *uint64
	if tenantID > 0 {
		value := uint64(tenantID)
		tid = &value
	}

	quota, err := g.quotas.GetQuota(ctx, &uid, tid)
	if err != nil || quota == nil || quota.Enabled == 0 {
		return nil
	}

	now := g.now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := g.check(ctx, quota, &uid, tid, startOfDay, startOfDay.Add(24*time.Hour), quotaWindow{
		label:    "daily",
		requests: quota.DailyRequestLimit,
		tokens:   quota.DailyTokenLimit,
		cost:     quota.DailyCostLimit,
	}); err != nil {
		return err
	}

	if quota.MonthlyRequestLimit == nil && quota.MonthlyTokenLimit == nil && quota.MonthlyCostLimit == nil {
		return nil
	}
	startOfMonth := monthlyWindowStart(now, quota.ResetDayOfMonth)
	return g.check(ctx, quota, &uid, tid, startOfMonth, startOfMonth.AddDate(0, 1, 0), quotaWindow{
		label:    "monthly",
		requests: quota.MonthlyRequestLimit,
		tokens:   quota.MonthlyTokenLimit,
		cost:     quota.MonthlyCostLimit,
	})
}

type quotaWindow struct {
	label    string
	requests *int
	tokens   *int
	cost     *float64
}

func (g *HermesQuotaGuard) check(
	ctx context.Context,
	_ *localmodel.AIQuota,
	userID, tenantID *uint64,
	from, to time.Time,
	window quotaWindow,
) error {
	summary, err := g.usage.GetUsageSummary(ctx, from, to, userID, tenantID)
	if err != nil || summary == nil {
		return nil
	}
	if window.requests != nil && summary.TotalRequests >= *window.requests {
		return fmt.Errorf("%w: %s request limit %d/%d", ErrHermesQuotaExceeded, window.label, summary.TotalRequests, *window.requests)
	}
	if window.tokens != nil && summary.TotalTokens >= *window.tokens {
		return fmt.Errorf("%w: %s token limit %d/%d", ErrHermesQuotaExceeded, window.label, summary.TotalTokens, *window.tokens)
	}
	if window.cost != nil && summary.TotalCost >= *window.cost {
		return fmt.Errorf("%w: %s cost limit %.2f/%.2f", ErrHermesQuotaExceeded, window.label, summary.TotalCost, *window.cost)
	}
	return nil
}

// monthlyWindowStart 按配额配置的重置日算出当前计费月的起点。
// 重置日钳制在 1-28，避免 29-31 在短月里落空。
func monthlyWindowStart(now time.Time, resetDay int) time.Time {
	if resetDay < 1 {
		resetDay = 1
	}
	if resetDay > 28 {
		resetDay = 28
	}
	if now.Day() >= resetDay {
		return time.Date(now.Year(), now.Month(), resetDay, 0, 0, 0, 0, now.Location())
	}
	return time.Date(now.Year(), now.Month()-1, resetDay, 0, 0, 0, 0, now.Location())
}
