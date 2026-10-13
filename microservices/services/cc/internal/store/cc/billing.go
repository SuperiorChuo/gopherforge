package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 计费三张表（实验线，随 cc-service AutoMigrate 建表，毕业后迁 goose）。
// 金额一律以「分」存 int64（与 CRM 同约定）；避免浮点误差。

// CCRate 费率：按被叫号码「最长前缀」匹配。未命中任何费率 = 免费不计费/不拦截。
type CCRate struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_cc_rates_tenant_prefix" json:"tenant_id"`
	// Prefix 被叫前缀，最长匹配（如 "0"=国内长途、"00"=国际、"1"=手机）。
	Prefix string `gorm:"size:32;not null;uniqueIndex:ux_cc_rates_tenant_prefix" json:"prefix"`
	Name   string `gorm:"size:64" json:"name"` // 说明，如"国内长途"
	// RatePerMin 每分钟费率（分）。
	RatePerMin int64 `gorm:"not null" json:"rate_per_min"`
	// BillingStep 计费步长（秒），默认 60；不足一步进按一步计。
	BillingStep int `gorm:"not null;default:60" json:"billing_step"`
	// ConnectFee 接通费（分），默认 0。
	ConnectFee int64     `gorm:"not null;default:0" json:"connect_fee"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (CCRate) TableName() string { return "cc_rates" }

// CCBillingAccount 租户余额账户（每租户唯一）。
type CCBillingAccount struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;uniqueIndex" json:"tenant_id"`
	// Balance 余额（分），可负（透支）。
	Balance int64 `gorm:"not null;default:0" json:"balance"`
	// CreditLimit 透支额度（分），默认 0。可用额 = Balance + CreditLimit。
	CreditLimit int64     `gorm:"not null;default:0" json:"credit_limit"`
	Status      string    `gorm:"size:16;not null;default:normal" json:"status"` // normal / frozen
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (CCBillingAccount) TableName() string { return "cc_billing_accounts" }

// CCCallCharge 话费流水（按 call_id 幂等，冲突忽略）。
type CCCallCharge struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	CallID   string `gorm:"size:64;uniqueIndex" json:"call_id"`
	Caller   string `gorm:"size:64" json:"caller"`
	Callee   string `gorm:"size:64" json:"callee"`
	BillSec  int    `json:"bill_sec"`
	RateID   uint64 `json:"rate_id"`
	// Amount 本次扣费（分）。
	Amount int64 `gorm:"not null" json:"amount"`
	// BalanceAfter 扣费后余额（分）。
	BalanceAfter int64     `gorm:"not null" json:"balance_after"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (CCCallCharge) TableName() string { return "cc_call_charges" }

// ---- 费率 CRUD ----

func (s *Store) ListRates(ctx context.Context, tenantID uint64) ([]CCRate, error) {
	// 按前缀长度降序（长前缀优先），便于前端展示与最长匹配语义一致。
	return fullread.All[CCRate](s.db.WithContext(ctx).
		Where("tenant_id = ?", EnsureTenant(tenantID)).
		Order("length(prefix) DESC, prefix ASC"))
}

// UpsertRate 按 (tenant, prefix) 幂等 upsert（无 ID 视为新建，有 ID 视为编辑）。
func (s *Store) UpsertRate(ctx context.Context, r *CCRate) (*CCRate, error) {
	r.TenantID = EnsureTenant(r.TenantID)
	r.Prefix = strings.TrimSpace(r.Prefix)
	if r.Prefix == "" {
		return nil, errors.New("prefix required")
	}
	if r.BillingStep <= 0 {
		r.BillingStep = 60
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing CCRate
		err := tx.Where("tenant_id = ? AND prefix = ?", r.TenantID, r.Prefix).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(r).Error
		case err != nil:
			return err
		}
		r.ID = existing.ID
		return tx.Model(&CCRate{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"name":         r.Name,
			"rate_per_min": r.RatePerMin,
			"billing_step": r.BillingStep,
			"connect_fee":  r.ConnectFee,
			"updated_at":   time.Now(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	var out CCRate
	if err := s.db.WithContext(ctx).First(&out, r.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) DeleteRate(ctx context.Context, tenantID, id uint64) (bool, error) {
	res := s.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", EnsureTenant(tenantID), id).
		Delete(&CCRate{})
	return res.RowsAffected > 0, res.Error
}

// MatchRate 最长前缀匹配：拉全量费率，内存里挑「callee 以其 prefix 开头且最长」的一条。
// 未命中返回 (nil, nil)。费率表规模小（每租户几十条），内存匹配足够。
func (s *Store) MatchRate(ctx context.Context, tenantID uint64, callee string) (*CCRate, error) {
	callee = strings.TrimSpace(callee)
	if callee == "" {
		return nil, nil
	}
	rates, err := s.ListRates(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// ListRates 已按前缀长度降序；再兜底排一次防止依赖 SQL length() 行为差异。
	sort.SliceStable(rates, func(i, j int) bool {
		return len(rates[i].Prefix) > len(rates[j].Prefix)
	})
	for i := range rates {
		if strings.HasPrefix(callee, rates[i].Prefix) {
			return &rates[i], nil
		}
	}
	return nil, nil
}

// ---- 账户 ----

// GetOrCreateAccount 取租户账户，不存在则建默认账户（余额 0、额度 0、normal）。
func (s *Store) GetOrCreateAccount(ctx context.Context, tenantID uint64) (*CCBillingAccount, error) {
	tenantID = EnsureTenant(tenantID)
	var acc CCBillingAccount
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&acc).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		acc = CCBillingAccount{TenantID: tenantID, Status: "normal"}
		// 并发下两路都建：audittrail 插件拒绝 ON CONFLICT 形状，改普通 Create，
		// 唯一约束兜底，冲突时重查拿已存在那行。gorm 未开 TranslateError，
		// ErrDuplicatedKey 不会产生，需按 pgx 错误码识别。
		if cerr := s.db.WithContext(ctx).Create(&acc).Error; cerr != nil && !isUniqueViolation(cerr) {
			return nil, cerr
		}
		if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&acc).Error; err != nil {
			return nil, err
		}
		return &acc, nil
	case err != nil:
		return nil, err
	}
	return &acc, nil
}

// isUniqueViolation 在驱动未把错误翻译成 gorm.ErrDuplicatedKey 时退化为文本匹配
// （同 ai 服务先例）：Postgres 报 "duplicate key value"，sqlite 报
// "UNIQUE constraint failed"，两者都认。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

// CCAccountTxn 账户资金流水（充值/调减留痕；话费扣减在 cc_call_charges，不重复记）。
type CCAccountTxn struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// Kind recharge=充值（正金额）/ adjust=调减（负金额）
	Kind string `gorm:"size:16;not null" json:"kind"`
	// Amount 有符号（分）：正=入账、负=调减。
	Amount int64 `gorm:"not null" json:"amount"`
	// BalanceAfter 本笔完成后的余额（分）。
	BalanceAfter int64 `gorm:"not null" json:"balance_after"`
	// OperatorID 操作人 user_id（0=系统）。
	OperatorID uint64    `json:"operator_id"`
	Remark     string    `gorm:"size:256" json:"remark"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func (CCAccountTxn) TableName() string { return "cc_account_txns" }

// Recharge 充值/调减余额（amountFen 正=充值、负=调减），事务内加减、写资金流水
// 并回读。operatorID/remark 进流水留痕（审计）。
func (s *Store) Recharge(ctx context.Context, tenantID uint64, amountFen int64, operatorID uint64, remark string) (*CCBillingAccount, error) {
	tenantID = EnsureTenant(tenantID)
	if _, err := s.GetOrCreateAccount(ctx, tenantID); err != nil {
		return nil, err
	}
	kind := "recharge"
	if amountFen < 0 {
		kind = "adjust"
	}
	var acc CCBillingAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先取主键再按 id 更新——audittrail 插件要求更新谓词含主键。
		if err := tx.Where("tenant_id = ?", tenantID).First(&acc).Error; err != nil {
			return err
		}
		if err := tx.Model(&CCBillingAccount{}).
			Where("id = ?", acc.ID).
			Updates(map[string]any{
				"balance":    gorm.Expr("balance + ?", amountFen),
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		// 重查拿更新后的余额（原子加已在 DB 生效）。
		if err := tx.First(&acc, acc.ID).Error; err != nil {
			return err
		}
		return tx.Create(&CCAccountTxn{
			TenantID: tenantID, Kind: kind, Amount: amountFen,
			BalanceAfter: acc.Balance, OperatorID: operatorID, Remark: remark,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// ListAccountTxns 分页列资金流水（tenantID 必填，按租户隔离）。
func (s *Store) ListAccountTxns(ctx context.Context, tenantID uint64, page, pageSize int) ([]CCAccountTxn, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	db := s.db.WithContext(ctx).Model(&CCAccountTxn{}).Where("tenant_id = ?", EnsureTenant(tenantID))
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []CCAccountTxn
	err := db.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// SetAccountStatus 冻结/解冻账户。
func (s *Store) SetAccountStatus(ctx context.Context, tenantID uint64, status string) (*CCBillingAccount, error) {
	tenantID = EnsureTenant(tenantID)
	acc, err := s.GetOrCreateAccount(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// 按主键更新——audittrail 插件要求更新谓词含主键。
	if err := s.db.WithContext(ctx).Model(&CCBillingAccount{}).
		Where("id = ?", acc.ID).
		Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	// 重查拿更新后的状态与时间戳（避免返回陈旧 UpdatedAt）。
	var updated CCBillingAccount
	if err := s.db.WithContext(ctx).First(&updated, acc.ID).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

// ---- 话费入账 ----

// ChargeCall 事务入账：插流水（call_id 冲突忽略=幂等）+ 扣余额 + 回填扣后余额。
// 幂等：同一 call_id 重复入账时插入被忽略，余额不再扣；返回已存在的那条流水。
func (s *Store) ChargeCall(ctx context.Context, charge *CCCallCharge) (*CCCallCharge, error) {
	charge.TenantID = EnsureTenant(charge.TenantID)
	if charge.CallID == "" {
		return nil, errors.New("call_id required")
	}
	if _, err := s.GetOrCreateAccount(ctx, charge.TenantID); err != nil {
		return nil, err
	}
	if charge.CreatedAt.IsZero() {
		charge.CreatedAt = time.Now()
	}
	var out CCCallCharge
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先查是否已入账（幂等短路，避免重复扣费）。
		var existing CCCallCharge
		err := tx.Where("call_id = ?", charge.CallID).First(&existing).Error
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 扣余额并回读扣后余额。
		if charge.Amount != 0 {
			if err := tx.Model(&CCBillingAccount{}).
				Where("tenant_id = ?", charge.TenantID).
				Updates(map[string]any{
					"balance":    gorm.Expr("balance - ?", charge.Amount),
					"updated_at": time.Now(),
				}).Error; err != nil {
				return err
			}
		}
		var acc CCBillingAccount
		if err := tx.Where("tenant_id = ?", charge.TenantID).First(&acc).Error; err != nil {
			return err
		}
		charge.BalanceAfter = acc.Balance
		// 插流水；并发下另一路可能已插，冲突忽略后回查。
		res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "call_id"}}, DoNothing: true}).Create(charge)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 冲突被忽略：这次没插进去，说明并发另一路已入账；这里的扣款要回滚。
			// 直接回退整个事务以避免重复扣费。
			return errIdempotentConflict
		}
		out = *charge
		return nil
	})
	if errors.Is(err, errIdempotentConflict) {
		// 重查已有流水返回（不再扣费）。
		if e := s.db.WithContext(ctx).Where("call_id = ?", charge.CallID).First(&out).Error; e != nil {
			return nil, e
		}
		return &out, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

var errIdempotentConflict = errors.New("charge already exists (idempotent)")

// ---- 流水查询 / 消费统计 ----

type ListChargesQuery struct {
	TenantID uint64
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

func (s *Store) ListCharges(ctx context.Context, q ListChargesQuery) ([]CCCallCharge, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}
	db := s.db.WithContext(ctx).Model(&CCCallCharge{}).Where("tenant_id = ?", EnsureTenant(q.TenantID))
	if q.From != nil {
		db = db.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("created_at <= ?", *q.To)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []CCCallCharge
	err := db.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&list).Error
	return list, total, err
}

// SumCharges 汇总 [from, to) 区间内的扣费合计（分）。
func (s *Store) SumCharges(ctx context.Context, tenantID uint64, from, to time.Time) (int64, error) {
	var sum *int64
	err := s.db.WithContext(ctx).Model(&CCCallCharge{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ?", EnsureTenant(tenantID), from, to).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	if sum == nil {
		return 0, nil
	}
	return *sum, nil
}
