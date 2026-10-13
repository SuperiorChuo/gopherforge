package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
)

// BalanceWarnOptions 余额预警/自动冻结循环的配置（main.go 从 env 装配）。
type BalanceWarnOptions struct {
	// ThresholdFen 预警阈值（分）：可用额 ≤ 此值的 normal 账户发预警。
	ThresholdFen int64
	// WarnUserID 站内信收件人（平台管理员 user_id）。
	WarnUserID uint64
	// AutoFreeze 欠费自动冻结开关（默认关）：可用额 ≤ 0 的 normal 账户自动冻结
	// 并联动停用其名下线路。小时级扫描即天然的宽限窗口。
	AutoFreeze bool
}

// syncTenantLineState 冻结/解冻联动媒体面：批量停用/恢复该租户名下线路
// （control-api /v1/tenants/:id/gateways/state）。返回受影响线路名。
// FS 未配置返回 (nil, nil)——联动静默跳过，冻结本身仍生效。
func (s *Server) syncTenantLineState(ctx context.Context, tenantID uint64, frozen bool) ([]string, error) {
	if !s.FS.Enabled() {
		return nil, nil
	}
	out, err := s.FS.Post(ctx, fmt.Sprintf("/v1/tenants/%d/gateways/state", tenantID), gin.H{"disabled": frozen})
	if err != nil {
		return nil, err
	}
	var names []string
	if raw, ok := out["affected"].([]any); ok {
		for _, v := range raw {
			if name, ok := v.(string); ok {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// RunBalanceWarnLoop 余额预警循环：启动 1 分钟后首扫，此后每小时一轮。
// 每轮两件事：①可用额 ≤ 阈值的 normal 账户站内信预警（notify ref 按天去重，
// 每租户每天一条；notify 未配置只记日志）；②开了 AutoFreeze 时，可用额 ≤ 0
// 的 normal 账户自动冻结 + 联动停用线路 + 通知。
func (s *Server) RunBalanceWarnLoop(ctx context.Context, notify *notifyclient.Client, opts BalanceWarnOptions) {
	log.Printf("cc balance-warn: enabled threshold=%d fen notify=%v auto_freeze=%v notify_user=%d",
		opts.ThresholdFen, notify.Enabled(), opts.AutoFreeze, opts.WarnUserID)
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		start := time.Now()
		s.scanBalanceWarn(ctx, notify, opts)
		jobbeat.Report(s.Store.DB(), jobbeat.Run{
			Key: "cc.balance_warn", Service: "cc-service",
			Description: "余额预警/欠费自动冻结扫描", IntervalSec: 3600, StartedAt: start,
		})
		timer.Reset(time.Hour)
	}
}

func (s *Server) scanBalanceWarn(ctx context.Context, notify *notifyclient.Client, opts BalanceWarnOptions) {
	accounts, err := s.Store.ListWarnAccounts(ctx, opts.ThresholdFen)
	if err != nil {
		log.Printf("cc balance-warn: scan failed: %v", err)
		return
	}
	today := time.Now().Format("2006-01-02")
	for _, acc := range accounts {
		avail := acc.Balance + acc.CreditLimit

		// 欠费自动冻结：可用额耗尽 → 冻结 + 联动断线 + 通知（独立 ref 去重）。
		if opts.AutoFreeze && avail <= 0 {
			if _, err := s.Store.SetAccountStatus(ctx, acc.TenantID, "frozen"); err != nil {
				log.Printf("cc auto-freeze: tenant=%d freeze failed: %v", acc.TenantID, err)
				continue
			}
			lines, err := s.syncTenantLineState(ctx, acc.TenantID, true)
			if err != nil {
				log.Printf("cc auto-freeze: tenant=%d line sync failed (frozen anyway): %v", acc.TenantID, err)
			}
			log.Printf("cc auto-freeze: tenant=%d avail=%d fen frozen, lines disabled: %v", acc.TenantID, avail, lines)
			if _, err := notify.Send(ctx, notifyclient.SendInput{
				TenantID: 1,
				UserID:   opts.WarnUserID,
				Type:     "cc_billing",
				RefType:  "cc_auto_freeze",
				RefID:    fmt.Sprintf("t%d-%s", acc.TenantID, today),
				Title:    fmt.Sprintf("话务账户欠费已自动冻结：租户 %d", acc.TenantID),
				Content: fmt.Sprintf("租户 %d 可用额 %.2f 元已耗尽，账户已自动冻结、名下 %d 条线路已停用。充值后请到「呼叫中心→计费管理→租户总览」解冻恢复。",
					acc.TenantID, float64(avail)/100, len(lines)),
				Link: "/cc/billing",
			}); err != nil {
				log.Printf("cc auto-freeze: notify tenant=%d failed: %v", acc.TenantID, err)
			}
			continue // 已冻结不再发低额预警
		}

		res, err := notify.Send(ctx, notifyclient.SendInput{
			TenantID: 1, // 通知发给平台侧管理员
			UserID:   opts.WarnUserID,
			Type:     "cc_billing",
			RefType:  "cc_balance_warn",
			RefID:    fmt.Sprintf("t%d-%s", acc.TenantID, today),
			Title:    fmt.Sprintf("话务账户余额预警：租户 %d", acc.TenantID),
			Content: fmt.Sprintf("租户 %d 话务账户可用额仅剩 %.2f 元（余额 %.2f + 透支额度 %.2f），请关注催缴或冻结。",
				acc.TenantID, float64(avail)/100, float64(acc.Balance)/100, float64(acc.CreditLimit)/100),
			Link: "/cc/billing",
		})
		if err != nil {
			log.Printf("cc balance-warn: notify tenant=%d failed: %v", acc.TenantID, err)
			continue
		}
		if res.Created > 0 {
			log.Printf("cc balance-warn: tenant=%d avail=%d fen notified", acc.TenantID, avail)
		}
	}
}

// RunHousekeepingLoop 日常维护循环：启动 2 分钟后首轮，此后每 24h 一轮。
// ①清理 keepDays 天前的 cc_webhook_logs；②每月 1 号自动出上月账单并站内信
// 通知平台管理员（notify ref 按账期去重——重启/多轮不重复发）。
func (s *Server) RunHousekeepingLoop(ctx context.Context, notify *notifyclient.Client, warnUserID uint64, keepDays int) {
	if keepDays <= 0 {
		keepDays = 30
	}
	log.Printf("cc housekeeping: enabled webhook_log_keep=%dd auto_bill=monthly", keepDays)
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		start := time.Now()
		s.runHousekeeping(ctx, notify, warnUserID, keepDays)
		jobbeat.Report(s.Store.DB(), jobbeat.Run{
			Key: "cc.housekeeping", Service: "cc-service",
			Description: "webhook 日志清理 + 每月自动出账", IntervalSec: 86400, StartedAt: start,
		})
		timer.Reset(24 * time.Hour)
	}
}

func (s *Server) runHousekeeping(ctx context.Context, notify *notifyclient.Client, warnUserID uint64, keepDays int) {
	if n, err := s.Store.PurgeWebhookLogs(ctx, time.Now().AddDate(0, 0, -keepDays)); err != nil {
		log.Printf("cc housekeeping: purge webhook logs failed: %v", err)
	} else if n > 0 {
		log.Printf("cc housekeeping: purged %d webhook logs (> %dd)", n, keepDays)
	}
	// 外呼尝试只服务滚动频次，保留 2 天足够对账排障。
	if n, err := s.Store.PurgeRiskAttempts(ctx, time.Now().Add(-48*time.Hour)); err != nil {
		log.Printf("cc housekeeping: purge risk attempts failed: %v", err)
	} else if n > 0 {
		log.Printf("cc housekeeping: purged %d risk attempts (> 48h)", n)
	}

	// 每月 1 号出上月账单（GenerateBills 幂等覆盖重算，多跑无害）。
	now := time.Now()
	if now.Day() != 1 {
		return
	}
	period := now.AddDate(0, -1, 0).Format("2006-01")
	n, err := s.Store.GenerateBills(ctx, period)
	if err != nil {
		log.Printf("cc housekeeping: auto bill %s failed: %v", period, err)
		return
	}
	log.Printf("cc housekeeping: auto bill %s generated for %d tenants", period, n)
	if n == 0 {
		return
	}
	if _, err := notify.Send(ctx, notifyclient.SendInput{
		TenantID: 1,
		UserID:   warnUserID,
		Type:     "cc_billing",
		RefType:  "cc_bill_gen",
		RefID:    period,
		Title:    fmt.Sprintf("话务月账单已出账：%s", period),
		Content:  fmt.Sprintf("账期 %s 已自动出账，共 %d 个租户。可到「呼叫中心→计费管理→月账单」查看并导出明细。", period, n),
		Link:     "/cc/billing",
	}); err != nil {
		log.Printf("cc housekeeping: bill notify failed: %v", err)
	}
}
