// Package store 持久化支付订单/退款/渠道（AutoMigrate 实验线）。
// 钱上的规矩（设计文档 §3.2，全部事务内强制）：
//   - 终态不可逆（paid/closed/refunded/failed 后拒绝任何状态写入）
//   - postgres 下状态迁移全程 SELECT ... FOR UPDATE 行锁（sqlite 单连接串行免锁）
//   - 退款累计 ≤ 实收，事务内校验
//   - 回调幂等：重复通知在已达目标态时静默成功
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/go-admin-kit/services/pay/internal/model"
	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 租户渠道是配置面，正常只有 mock/微信/支付宝等少数几条。
// 加硬上限只为封住异常写入把收银台一次拉爆的尾部风险。
const maxChannelsPerTenant = 50

var (
	ErrChannelExists    = errors.New("同类型渠道已存在")
	ErrChannelDisabled  = errors.New("渠道未启用")
	ErrOrderNotFound    = errors.New("订单不存在")
	ErrOrderTerminal    = errors.New("订单已终态")
	ErrOrderNotPaid     = errors.New("订单未支付")
	ErrAmountInvalid    = errors.New("金额必须为正整数（分）")
	ErrRefundExceeds    = errors.New("退款累计超出实收金额")
	ErrRefundTerminal   = errors.New("退款单已终态")
	ErrAmountMismatch   = errors.New("回调金额与订单不符")
	ErrTokenExpired     = errors.New("收银台链接已失效")
	ErrDuplicateApplied = errors.New("重复通知（已处理）")
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db)
}

// NewWithDB wraps an existing gorm.DB (tests inject sqlite in-memory).
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := db.AutoMigrate(
		&model.Channel{}, &model.Order{}, &model.Refund{},
		&model.NotifyLog{}, &model.CallbackJob{},
	); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

// lockOrder 行锁取单（postgres FOR UPDATE；sqlite 串行免锁，照 bpm lockInstance 先例）。
func lockOrder(tx *gorm.DB, tenantID uint64, orderNo string) (*model.Order, error) {
	q := tx.Where("tenant_id = ? AND order_no = ?", tenantID, orderNo)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var o model.Order
	if err := q.First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &o, nil
}

func orderTerminal(status string) bool {
	return status == model.OrderPaid || status == model.OrderClosed
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newOrderNo() string  { return "P" + time.Now().Format("20060102150405") + randomHex(4) }
func newRefundNo() string { return "R" + time.Now().Format("20060102150405") + randomHex(4) }

// ---- 渠道 ----

func (s *Store) ListChannels(tenantID uint64) ([]model.Channel, error) {
	var out []model.Channel
	err := s.db.Where("tenant_id = ?", tenantID).Order("id").Limit(maxChannelsPerTenant).Find(&out).Error
	return out, err
}

func (s *Store) CreateChannel(ctx context.Context, ch *model.Channel) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cnt int64
		if err := tx.Model(&model.Channel{}).Where("tenant_id = ? AND code = ?", ch.TenantID, ch.Code).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrChannelExists
		}
		return tx.Create(ch).Error
	})
}

func (s *Store) UpdateChannel(ctx context.Context, tenantID, id uint64, name, remark string, status int8, feeRateBps int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&model.Channel{}).
			Where("tenant_id = ? AND id = ?", tenantID, id).
			Updates(map[string]any{"name": name, "remark": remark, "status": status, "fee_rate_bps": feeRateBps}).Error
	})
}

func (s *Store) DeleteChannel(ctx context.Context, tenantID, id uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&model.Channel{}).Error
	})
}

// EnabledChannel 取启用态渠道（收银台拉起支付前校验）。
func (s *Store) EnabledChannel(tenantID uint64, code string) (*model.Channel, error) {
	var ch model.Channel
	err := s.db.Where("tenant_id = ? AND code = ? AND status = 1", tenantID, code).First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelDisabled
	}
	return &ch, err
}

// ---- 订单 ----

// CreateOrder 业务方建单（internal 端点）。幂等锚点 (tenant,biz_type,biz_id)：
// 已有未终态单直接返回；已 paid 返回该单（业务侧自行判断）；closed 则新开一单。
func (s *Store) CreateOrder(ctx context.Context, tenantID uint64, bizType, bizID, subject string, amountCents int64, expireIn time.Duration) (*model.Order, error) {
	if amountCents <= 0 {
		return nil, ErrAmountInvalid
	}
	var out *model.Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Order
		err := tx.Where("tenant_id = ? AND biz_type = ? AND biz_id = ?", tenantID, bizType, bizID).
			First(&existing).Error
		switch {
		case err == nil:
			if existing.Status != model.OrderClosed {
				out = &existing
				return nil
			}
			// closed 旧单让位：置换 biz_id 锚点，允许重新收款
			if err := tx.Model(&existing).Update("biz_id", existing.BizID+"#closed-"+randomHex(3)).Error; err != nil {
				return err
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		o := &model.Order{
			TenantID: tenantID, OrderNo: newOrderNo(), OrderToken: randomHex(24),
			BizType: bizType, BizID: bizID, Subject: subject,
			AmountCents: amountCents, Status: model.OrderWaiting,
			ExpireAt: time.Now().Add(expireIn),
		}
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}

// OrderByToken 收银台取单（匿名，token 即凭据；终态/过期单仍可读用于展示结果页）。
func (s *Store) OrderByToken(token string) (*model.Order, error) {
	var o model.Order
	err := s.db.Where("order_token = ?", token).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	return &o, err
}

// MarkPaying 收银台选渠道拉起支付：waiting/paying → paying（可换渠道重拉）。
func (s *Store) MarkPaying(ctx context.Context, tenantID uint64, orderNo string, channelID uint64, channelCode string) (*model.Order, error) {
	var out *model.Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, err := lockOrder(tx, tenantID, orderNo)
		if err != nil {
			return err
		}
		if orderTerminal(o.Status) {
			return ErrOrderTerminal
		}
		if time.Now().After(o.ExpireAt) {
			return ErrTokenExpired
		}
		o.Status = model.OrderPaying
		o.ChannelID = channelID
		o.ChannelCode = channelCode
		if err := tx.Model(o).Updates(map[string]any{
			"status": o.Status, "channel_id": channelID, "channel_code": channelCode,
		}).Error; err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}

// ApplyPaid 渠道回调/查单补偿驱动 → paid 终态 + 排业务回调任务。
// 幂等：已 paid 且交易号一致 → ErrDuplicateApplied（调用方 ACK 成功）；
// 金额不符 → ErrAmountMismatch（只告警不改单，调用方记 notify_log）。
func (s *Store) ApplyPaid(ctx context.Context, tenantID uint64, orderNo, channelTradeNo string, amountCents int64) (*model.Order, error) {
	var out *model.Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, err := lockOrder(tx, tenantID, orderNo)
		if err != nil {
			return err
		}
		if o.Status == model.OrderPaid {
			out = o
			return ErrDuplicateApplied
		}
		if o.Status == model.OrderClosed {
			return ErrOrderTerminal
		}
		if amountCents != o.AmountCents {
			return ErrAmountMismatch
		}
		now := time.Now()
		if err := tx.Model(o).Updates(map[string]any{
			"status": model.OrderPaid, "channel_trade_no": channelTradeNo, "paid_at": now,
		}).Error; err != nil {
			return err
		}
		o.Status = model.OrderPaid
		o.ChannelTradeNo = channelTradeNo
		o.PaidAt = &now
		if err := tx.Create(&model.CallbackJob{
			TenantID: tenantID, OrderID: o.ID, Kind: "order", NextAt: now,
		}).Error; err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}

// CloseOrder 主动关单/过期关单：waiting/paying → closed。
func (s *Store) CloseOrder(ctx context.Context, tenantID uint64, orderNo string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, err := lockOrder(tx, tenantID, orderNo)
		if err != nil {
			return err
		}
		if orderTerminal(o.Status) {
			return ErrOrderTerminal
		}
		return tx.Model(o).Update("status", model.OrderClosed).Error
	})
}

// ExpireDueOrders 补偿 cron：过期未支付单批量关单（SKIP LOCKED 抢单，多副本安全）。
func (s *Store) ExpireDueOrders(limit int) (int, error) {
	closed := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("status IN ? AND expire_at < ?", []string{model.OrderWaiting, model.OrderPaying}, time.Now()).
			Order("id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		var due []model.Order
		if err := q.Find(&due).Error; err != nil {
			return err
		}
		if len(due) == 0 {
			return nil
		}
		ids := make([]uint64, len(due))
		for i := range due {
			ids[i] = due[i].ID
		}
		result := tx.Model(&model.Order{}).Where("id IN ?", ids).Update("status", model.OrderClosed)
		closed = int(result.RowsAffected)
		return result.Error
	})
	return closed, err
}

func (s *Store) ListOrders(tenantID uint64, status, bizType string, page, pageSize int) ([]model.Order, int64, error) {
	q := s.db.Model(&model.Order{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if bizType != "" {
		q = q.Where("biz_type = ?", bizType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.Order
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}

func (s *Store) OrderByNo(tenantID uint64, orderNo string) (*model.Order, error) {
	var o model.Order
	err := s.db.Where("tenant_id = ? AND order_no = ?", tenantID, orderNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	return &o, err
}

func (s *Store) OrderByBiz(tenantID uint64, bizType, bizID string) (*model.Order, error) {
	var o model.Order
	err := s.db.Where("tenant_id = ? AND biz_type = ? AND biz_id = ?", tenantID, bizType, bizID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	return &o, err
}

// ---- 退款 ----

// CreateRefund 发起退款：订单须 paid，事务内校验累计 ≤ 实收。
// mock 渠道由调用方在同事务外立即 ApplyRefundResult；真渠道等回调。
func (s *Store) CreateRefund(ctx context.Context, tenantID uint64, orderNo string, amountCents int64, reason string, operator uint64) (*model.Refund, error) {
	if amountCents <= 0 {
		return nil, ErrAmountInvalid
	}
	var out *model.Refund
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, err := lockOrder(tx, tenantID, orderNo)
		if err != nil {
			return err
		}
		if o.Status != model.OrderPaid {
			return ErrOrderNotPaid
		}
		// 累计校验含在途（pending/processing）退款，防并发超退
		var inflight int64
		if err := tx.Model(&model.Refund{}).
			Where("order_id = ? AND status IN ?", o.ID, []string{model.RefundPending, model.RefundProcessing}).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&inflight).Error; err != nil {
			return err
		}
		if o.RefundedCents+inflight+amountCents > o.AmountCents {
			return ErrRefundExceeds
		}
		r := &model.Refund{
			TenantID: tenantID, RefundNo: newRefundNo(), OrderID: o.ID, OrderNo: o.OrderNo,
			AmountCents: amountCents, Reason: reason, Status: model.RefundProcessing,
			OperatorUserID: operator,
		}
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		out = r
		return nil
	})
	return out, err
}

// ApplyRefundResult 渠道退款结果 → 终态；成功则累计 refunded_cents + 排业务回调。
func (s *Store) ApplyRefundResult(ctx context.Context, tenantID uint64, refundNo, channelRefundNo string, succeeded bool) (*model.Refund, error) {
	var out *model.Refund
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var r model.Refund
		q := tx.Where("tenant_id = ? AND refund_no = ?", tenantID, refundNo)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&r).Error; err != nil {
			return err
		}
		if r.Status == model.RefundRefunded || r.Status == model.RefundFailed {
			out = &r
			return ErrDuplicateApplied
		}
		o, err := lockOrder(tx, tenantID, r.OrderNo)
		if err != nil {
			return err
		}
		now := time.Now()
		status := model.RefundFailed
		if succeeded {
			status = model.RefundRefunded
			if o.RefundedCents+r.AmountCents > o.AmountCents {
				return ErrRefundExceeds
			}
			if err := tx.Model(o).Update("refunded_cents", gorm.Expr("refunded_cents + ?", r.AmountCents)).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&r).Updates(map[string]any{
			"status": status, "channel_refund_no": channelRefundNo, "finished_at": now,
		}).Error; err != nil {
			return err
		}
		r.Status = status
		r.FinishedAt = &now
		if err := tx.Create(&model.CallbackJob{
			TenantID: tenantID, OrderID: o.ID, Kind: "refund", RefundID: r.ID, NextAt: now,
		}).Error; err != nil {
			return err
		}
		out = &r
		return nil
	})
	return out, err
}

func (s *Store) ListRefunds(tenantID uint64, status string, page, pageSize int) ([]model.Refund, int64, error) {
	q := s.db.Model(&model.Refund{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.Refund
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}

// ---- 回调任务与日志 ----

// ClaimCallbackJobs 抢占到期回调任务（SKIP LOCKED，多副本安全）：
// 取出即推迟 NextAt（悲观续租），执行结果由 Finish/Retry 落定。
func (s *Store) ClaimCallbackJobs(limit int, lease time.Duration) ([]model.CallbackJob, error) {
	var claimed []model.CallbackJob
	err := s.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("status = ? AND next_at <= ?", "pending", time.Now()).Order("id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&claimed).Error; err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		ids := make([]uint64, len(claimed))
		for i := range claimed {
			ids[i] = claimed[i].ID
		}
		return tx.Model(&model.CallbackJob{}).Where("id IN ?", ids).
			Update("next_at", time.Now().Add(lease)).Error
	})
	return claimed, err
}

func (s *Store) FinishCallbackJob(id uint64) error {
	return s.db.Delete(&model.CallbackJob{}, id).Error
}

// RetryCallbackJob 失败重试：指数退避，超过 maxAttempts 落 dead。
func (s *Store) RetryCallbackJob(id uint64, attempts int, maxAttempts int, lastErr string) error {
	if len(lastErr) > 480 {
		lastErr = lastErr[:480]
	}
	if attempts >= maxAttempts {
		return s.db.Model(&model.CallbackJob{}).Where("id = ?", id).
			Updates(map[string]any{"status": "dead", "attempts": attempts, "last_error": lastErr}).Error
	}
	backoff := time.Duration(1<<uint(attempts)) * 30 * time.Second // 30s 60s 120s ...
	return s.db.Model(&model.CallbackJob{}).Where("id = ?", id).
		Updates(map[string]any{"attempts": attempts, "next_at": time.Now().Add(backoff), "last_error": lastErr}).Error
}

// ReviveCallbackJob 管理面手工重推 dead 任务。
func (s *Store) ReviveCallbackJob(tenantID, id uint64) error {
	return s.db.Model(&model.CallbackJob{}).
		Where("tenant_id = ? AND id = ? AND status = 'dead'", tenantID, id).
		Updates(map[string]any{"status": "pending", "attempts": 0, "next_at": time.Now()}).Error
}

func (s *Store) ListCallbackJobs(tenantID uint64, status string, limit int) ([]model.CallbackJob, error) {
	q := s.db.Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []model.CallbackJob
	err := q.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (s *Store) AddNotifyLog(l *model.NotifyLog) error {
	if len(l.Body) > 8192 {
		l.Body = l.Body[:8192]
	}
	return s.db.Create(l).Error
}

func (s *Store) ListNotifyLogs(tenantID uint64, limit int) ([]model.NotifyLog, error) {
	var out []model.NotifyLog
	err := s.db.Where("tenant_id = ?", tenantID).Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// Stats 订单统计（管理页顶卡）。
func (s *Store) Stats(tenantID uint64) (map[string]any, error) {
	type row struct {
		Status string
		Cnt    int64
		Sum    int64
	}
	var rows []row
	if err := s.db.Model(&model.Order{}).
		Select("status, COUNT(*) AS cnt, COALESCE(SUM(amount_cents),0) AS sum").
		Where("tenant_id = ?", tenantID).Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]any{"waiting": int64(0), "paying": int64(0), "paid": int64(0), "closed": int64(0), "paid_amount_cents": int64(0)}
	for _, r := range rows {
		out[r.Status] = r.Cnt
		if r.Status == model.OrderPaid {
			out["paid_amount_cents"] = r.Sum
		}
	}
	var refunded int64
	if err := s.db.Model(&model.Refund{}).Where("tenant_id = ? AND status = ?", tenantID, model.RefundRefunded).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&refunded).Error; err != nil {
		return nil, err
	}
	out["refunded_amount_cents"] = refunded
	return out, nil
}
