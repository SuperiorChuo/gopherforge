package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 平台侧账务：全租户总览、月账单出账。仅供平台租户（tenant=1）的管理端点调用，
// 权限闸门在 API 层（requirePlatformAdmin）。

// 无分页查询的硬上限：这几个接口的返回全量进内存再序列化，租户数/账单数/单月
// 流水在正常量级下远低于上限，加限只为封住"数据涨到异常量级时一次查询打爆
// 256M 内存限额"的尾部风险。触顶时截断而非报错（返回 truncated 提示见 API 层）。
const (
	maxOverviewAccounts = 2000
	maxWarnAccounts     = 2000
	maxBillsRows        = 5000
	maxChargesExport    = 100000
)

// AccountOverviewRow 总览一行：账户 + 消费聚合。
type AccountOverviewRow struct {
	CCBillingAccount
	MonthSpent int64 `json:"month_spent"` // 分
	TodaySpent int64 `json:"today_spent"` // 分
}

// ListAccountsOverview 列全部租户账户并聚合本月/今日消费（分）。
// 账户在首次充值/入账时才建行——只出现在总览里的租户都是发生过账务动作的。
func (s *Store) ListAccountsOverview(ctx context.Context) ([]AccountOverviewRow, error) {
	var accounts []CCBillingAccount
	if err := s.db.WithContext(ctx).Order("tenant_id ASC").
		Limit(maxOverviewAccounts).Find(&accounts).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	sumSince := func(since time.Time) (map[uint64]int64, error) {
		type row struct {
			TenantID uint64
			Sum      int64
		}
		var rows []row
		err := s.db.WithContext(ctx).Model(&CCCallCharge{}).
			Select("tenant_id, COALESCE(SUM(amount),0) AS sum").
			Where("created_at >= ?", since).
			Group("tenant_id").Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		m := make(map[uint64]int64, len(rows))
		for _, r := range rows {
			m[r.TenantID] = r.Sum
		}
		return m, nil
	}
	monthByTenant, err := sumSince(monthStart)
	if err != nil {
		return nil, err
	}
	todayByTenant, err := sumSince(dayStart)
	if err != nil {
		return nil, err
	}

	out := make([]AccountOverviewRow, 0, len(accounts))
	for _, acc := range accounts {
		out = append(out, AccountOverviewRow{
			CCBillingAccount: acc,
			MonthSpent:       monthByTenant[acc.TenantID],
			TodaySpent:       todayByTenant[acc.TenantID],
		})
	}
	return out, nil
}

// ListWarnAccounts 返回可用额（余额+透支额度）低于阈值且未冻结的账户（余额预警）。
func (s *Store) ListWarnAccounts(ctx context.Context, thresholdFen int64) ([]CCBillingAccount, error) {
	var list []CCBillingAccount
	err := s.db.WithContext(ctx).
		Where("balance + credit_limit <= ? AND status = ?", thresholdFen, "normal").
		Order("tenant_id ASC").
		Limit(maxWarnAccounts).
		Find(&list).Error
	return list, err
}

// ---- 月账单 ----

// CCBill 月账单：按 (租户, 账期) 唯一，重复生成覆盖（重算语义）。
type CCBill struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;uniqueIndex:ux_cc_bills_tenant_period" json:"tenant_id"`
	// Period 账期 "YYYY-MM"。
	Period string `gorm:"size:7;not null;uniqueIndex:ux_cc_bills_tenant_period" json:"period"`
	// CallCount 计费通话数；TotalBillSec 计费时长合计（秒）；TotalAmount 金额合计（分）。
	CallCount    int       `json:"call_count"`
	TotalBillSec int64     `json:"total_bill_sec"`
	TotalAmount  int64     `json:"total_amount"`
	GeneratedAt  time.Time `json:"generated_at"`
	CreatedAt    time.Time `json:"created_at"`
}

func (CCBill) TableName() string { return "cc_bills" }

// PeriodRange 把 "YYYY-MM" 解析为 [monthStart, nextMonthStart)。
func PeriodRange(period string) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01", period, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("period must be YYYY-MM")
	}
	return t, t.AddDate(0, 1, 0), nil
}

// GenerateBills 按账期聚合话费流水出账（每个有流水的租户一行，upsert 覆盖重算）。
// 返回生成的账单数。
func (s *Store) GenerateBills(ctx context.Context, period string) (int, error) {
	from, to, err := PeriodRange(period)
	if err != nil {
		return 0, err
	}
	type agg struct {
		TenantID     uint64
		CallCount    int
		TotalBillSec int64
		TotalAmount  int64
	}
	var rows []agg
	if err := s.db.WithContext(ctx).Model(&CCCallCharge{}).
		Select("tenant_id, COUNT(*) AS call_count, COALESCE(SUM(bill_sec),0) AS total_bill_sec, COALESCE(SUM(amount),0) AS total_amount").
		Where("created_at >= ? AND created_at < ?", from, to).
		Group("tenant_id").Scan(&rows).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			bill := CCBill{
				TenantID: r.TenantID, Period: period,
				CallCount: r.CallCount, TotalBillSec: r.TotalBillSec, TotalAmount: r.TotalAmount,
				GeneratedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "period"}},
				DoUpdates: clause.Assignments(map[string]any{
					"call_count": r.CallCount, "total_bill_sec": r.TotalBillSec,
					"total_amount": r.TotalAmount, "generated_at": now,
				}),
			}).Create(&bill).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// ListBills 列账单：tenantID>0 只看该租户；0=全部（平台侧）。period 可空=全部账期。
func (s *Store) ListBills(ctx context.Context, tenantID uint64, period string) ([]CCBill, error) {
	db := s.db.WithContext(ctx).Model(&CCBill{})
	if tenantID > 0 {
		db = db.Where("tenant_id = ?", tenantID)
	}
	if period != "" {
		db = db.Where("period = ?", period)
	}
	var list []CCBill
	err := db.Order("period DESC, tenant_id ASC").Limit(maxBillsRows).Find(&list).Error
	return list, err
}

// GetBill 按 ID 取账单。未命中返回 gorm.ErrRecordNotFound。
func (s *Store) GetBill(ctx context.Context, id uint64) (*CCBill, error) {
	var b CCBill
	if err := s.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// ChargesInRange 拉某租户 [from, to) 的全部话费流水（账单明细导出用，不分页；
// 单租户单月量级可控）。
func (s *Store) ChargesInRange(ctx context.Context, tenantID uint64, from, to time.Time) ([]CCCallCharge, error) {
	var list []CCCallCharge
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", EnsureTenant(tenantID), from, to).
		Order("id ASC").Limit(maxChargesExport).Find(&list).Error
	return list, err
}

// ErrBillNotFound re-export 便于 API 层判断（避免直接依赖 gorm）。
var ErrBillNotFound = gorm.ErrRecordNotFound

// IsNotFound 判空辅助。
func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
