package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// AIQuotaChecker 配额检查器（用于限流）
type AIQuotaChecker struct {
	quotaService *service.AIQuotaService
	usageDAO     *dao.AIUsageLogDAO
	db           *gorm.DB
}

// NewAIQuotaChecker 创建配额检查器
func NewAIQuotaChecker(quotaService *service.AIQuotaService, usageDAO *dao.AIUsageLogDAO, db *gorm.DB) *AIQuotaChecker {
	return &AIQuotaChecker{
		quotaService: quotaService,
		usageDAO:     usageDAO,
		db:           db,
	}
}

// CheckQuota 返回配额检查中间件
func (c *AIQuotaChecker) CheckQuota() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 只拦截 AI 调用接口（chat/embed/image）
		if !isAICallEndpoint(ctx.Request.URL.Path) {
			ctx.Next()
			return
		}

		// 获取用户和租户信息
		userID, exists := ctx.Get("user_id")
		if !exists {
			ctx.Next()
			return
		}
		userIDUint := userID.(uint)
		userIDU64 := uint64(userIDUint)

		tenantID := tenant.FromContextOrDefault(ctx.Request.Context())
		var tenantIDU64 *uint64
		if tenantID > 0 {
			tid := uint64(tenantID)
			tenantIDU64 = &tid
		}

		// 获取适用的配额（user > tenant > global）
		quota, err := c.quotaService.GetQuota(ctx.Request.Context(), &userIDU64, tenantIDU64)
		if err != nil || quota == nil || quota.Enabled == 0 {
			// 没有配额或未启用，放行
			ctx.Next()
			return
		}

		// 检查每日配额
		if err := c.checkDailyQuota(ctx.Request.Context(), quota, &userIDU64, tenantIDU64); err != nil {
			ctx.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": err.Error(),
			})
			ctx.Abort()
			return
		}

		// 检查每月配额（如果配置了）
		if quota.MonthlyTokenLimit != nil || quota.MonthlyCostLimit != nil || quota.MonthlyRequestLimit != nil {
			if err := c.checkMonthlyQuota(ctx.Request.Context(), quota, &userIDU64, tenantIDU64); err != nil {
				ctx.JSON(http.StatusTooManyRequests, gin.H{
					"code":    429,
					"message": err.Error(),
				})
				ctx.Abort()
				return
			}
		}

		ctx.Next()
	}
}

// checkDailyQuota 检查每日配额
func (c *AIQuotaChecker) checkDailyQuota(ctx context.Context, quota *localmodel.AIQuota, userID, tenantID *uint64) error {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	summary, err := c.usageDAO.GetUsageSummary(ctx, startOfDay, endOfDay, userID, tenantID)
	if err != nil {
		return nil // 查询失败时不阻断，记录日志即可
	}

	// 检查每日请求次数
	if quota.DailyRequestLimit != nil && summary.TotalRequests >= *quota.DailyRequestLimit {
		return fmt.Errorf("daily request limit exceeded: %d/%d", summary.TotalRequests, *quota.DailyRequestLimit)
	}

	// 检查每日 token 用量
	if quota.DailyTokenLimit != nil && summary.TotalTokens >= *quota.DailyTokenLimit {
		return fmt.Errorf("daily token limit exceeded: %d/%d", summary.TotalTokens, *quota.DailyTokenLimit)
	}

	// 检查每日费用
	if quota.DailyCostLimit != nil && summary.TotalCost >= *quota.DailyCostLimit {
		return fmt.Errorf("daily cost limit exceeded: %.2f/%.2f", summary.TotalCost, *quota.DailyCostLimit)
	}

	return nil
}

// checkMonthlyQuota 检查每月配额
func (c *AIQuotaChecker) checkMonthlyQuota(ctx context.Context, quota *localmodel.AIQuota, userID, tenantID *uint64) error {
	now := time.Now()

	// 根据配置的 reset_day_of_month 计算月度起始日期
	resetDay := quota.ResetDayOfMonth
	if resetDay < 1 {
		resetDay = 1
	}
	if resetDay > 28 {
		resetDay = 28
	}

	var startOfMonth time.Time
	if now.Day() >= resetDay {
		// 当前月的重置日
		startOfMonth = time.Date(now.Year(), now.Month(), resetDay, 0, 0, 0, 0, now.Location())
	} else {
		// 上个月的重置日
		startOfMonth = time.Date(now.Year(), now.Month()-1, resetDay, 0, 0, 0, 0, now.Location())
	}
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	summary, err := c.usageDAO.GetUsageSummary(ctx, startOfMonth, endOfMonth, userID, tenantID)
	if err != nil {
		return nil
	}

	// 检查每月请求次数
	if quota.MonthlyRequestLimit != nil && summary.TotalRequests >= *quota.MonthlyRequestLimit {
		return fmt.Errorf("monthly request limit exceeded: %d/%d", summary.TotalRequests, *quota.MonthlyRequestLimit)
	}

	// 检查每月 token 用量
	if quota.MonthlyTokenLimit != nil && summary.TotalTokens >= *quota.MonthlyTokenLimit {
		return fmt.Errorf("monthly token limit exceeded: %d/%d", summary.TotalTokens, *quota.MonthlyTokenLimit)
	}

	// 检查每月费用
	if quota.MonthlyCostLimit != nil && summary.TotalCost >= *quota.MonthlyCostLimit {
		return fmt.Errorf("monthly cost limit exceeded: %.2f/%.2f", summary.TotalCost, *quota.MonthlyCostLimit)
	}

	return nil
}

// isAICallEndpoint 判断是否为 AI 调用接口
func isAICallEndpoint(path string) bool {
	aiCallPaths := []string{
		"/api/v1/ai/chat",
		"/api/v1/ai/compose",
		"/api/v1/ai/image",
		"/api/v1/ai/kb/search",
	}
	for _, p := range aiCallPaths {
		if path == p {
			return true
		}
	}
	return false
}
