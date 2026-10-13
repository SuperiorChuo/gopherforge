package store

import (
	"context"
	"fmt"
	"github.com/go-admin-kit/services/crm/internal/model"
	"sort"
	"time"
)

type StageBucket struct {
	Stage       string `json:"stage"`
	Count       int64  `json:"count"`
	AmountCents int64  `json:"amount_cents"`
}

// TrendBucket 月度趋势数据点（仪表盘折线图用）。
type TrendBucket struct {
	Month   string `json:"month"` // "2026-01"
	Leads   int64  `json:"leads"`
	Won     int64  `json:"won"`
	Revenue int64  `json:"revenue"` // amount_cents
}

type SalesStats struct {
	Funnel           []StageBucket `json:"funnel"`
	NewLeads         int64         `json:"new_leads"`
	ConvertedLeads   int64         `json:"converted_leads"`
	WonCount         int64         `json:"won_count"`
	WonAmountCents   int64         `json:"won_amount_cents"`
	ContractCents    int64         `json:"contract_cents"`
	PaidCents        int64         `json:"paid_cents"`
	CustomerCount    int64         `json:"customer_count"`
	OpenOpportunity  int64         `json:"open_opportunity"`
	FollowupsOverdue int64         `json:"followups_overdue"`
	Trend            []TrendBucket `json:"trend"`
	Owners           []OwnerStat   `json:"owners"`
}

// OwnerStat 销售业绩排行（M2）：按负责人聚合，赢单金额降序。
type OwnerStat struct {
	OwnerUserID    uint64 `json:"owner_user_id"`
	DisplayName    string `json:"display_name,omitempty"`
	WonCount       int64  `json:"won_count"`
	WonAmountCents int64  `json:"won_amount_cents"`
	OpenCount      int64  `json:"open_count"`
	ContractCents  int64  `json:"contract_cents"`
	PaidCents      int64  `json:"paid_cents"`
}

// ownerStats 按负责人聚合区间业绩；users 表取显示名是 best-effort
// （crm 与 system 同库的现实红利，测试库无 users 表时静默留空）。
func (s *Store) ownerStats(ctx context.Context, tenantID uint64, since time.Time) []OwnerStat {
	byOwner := map[uint64]*OwnerStat{}
	get := func(id uint64) *OwnerStat {
		if o, ok := byOwner[id]; ok {
			return o
		}
		o := &OwnerStat{OwnerUserID: id}
		byOwner[id] = o
		return o
	}
	type aggRow struct {
		Owner uint64
		N     int64
		Amt   int64
	}
	var rows []aggRow
	// 区间赢单
	if err := tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("stage = ? AND won_at >= ? AND owner_user_id > 0", "won", since).
		Select("owner_user_id AS owner, COUNT(*) AS n, COALESCE(SUM(amount_cents),0) AS amt").
		Group("owner_user_id").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			o := get(r.Owner)
			o.WonCount, o.WonAmountCents = r.N, r.Amt
		}
	}
	// 进行中商机（当前快照）
	rows = nil
	if err := tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("stage IN ? AND owner_user_id > 0", []string{"initial", "demand", "proposal", "negotiation"}).
		Select("owner_user_id AS owner, COUNT(*) AS n, 0 AS amt").
		Group("owner_user_id").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			get(r.Owner).OpenCount = r.N
		}
	}
	// 区间签约额（生效合同）
	rows = nil
	if err := tenantQ(s.db.Model(&model.Contract{}), tenantID).
		Where("status = ? AND created_at >= ? AND owner_user_id > 0", "active", since).
		Select("owner_user_id AS owner, COUNT(*) AS n, COALESCE(SUM(amount_cents),0) AS amt").
		Group("owner_user_id").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			get(r.Owner).ContractCents = r.Amt
		}
	}
	// 区间回款额（归属合同负责人；只计已确认，BPM M2 口径）
	rows = nil
	if err := s.db.Table("crm_payments AS p").
		Joins("JOIN crm_contracts c ON c.id = p.contract_id").
		Where("p.tenant_id = ? AND p.paid_at >= ? AND p.status = ? AND c.owner_user_id > 0",
			tenantID, since, PaymentConfirmed).
		Select("c.owner_user_id AS owner, COUNT(*) AS n, COALESCE(SUM(p.amount_cents),0) AS amt").
		Group("c.owner_user_id").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			get(r.Owner).PaidCents = r.Amt
		}
	}
	out := make([]OwnerStat, 0, len(byOwner))
	ids := make([]uint64, 0, len(byOwner))
	for id, o := range byOwner {
		out = append(out, *o)
		ids = append(ids, id)
	}
	// 显示名 best-effort（Phase 2C 走 identity owner API，失败静默降级空名）
	if len(ids) > 0 {
		m := s.displayNames(ctx, ids)
		for i := range out {
			out[i].DisplayName = m[out[i].OwnerUserID]
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WonAmountCents != out[j].WonAmountCents {
			return out[i].WonAmountCents > out[j].WonAmountCents
		}
		return out[i].ContractCents > out[j].ContractCents
	})
	return out
}

// Stats 汇总看板：漏斗为当前快照，其余按 since 起算。
func (s *Store) Stats(ctx context.Context, tenantID uint64, since time.Time) (*SalesStats, error) {
	out := &SalesStats{}
	// 漏斗一次分组聚合（原按阶段循环 COUNT+SUM = 12 次往返）。won/lost 终态
	// 按结单时间圈区间（won_at/lost_at 比 updated_at 精确），进行中阶段不圈时间，
	// 故用 CASE 内联区间条件而不是 WHERE——口径与原逐阶段 terminalFilter 一致。
	type funnelRow struct {
		Stage string
		Cnt   int64
		Cents int64
	}
	var frows []funnelRow
	if err := tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where(`stage NOT IN ? OR (stage = ? AND won_at >= ?) OR (stage = ? AND lost_at >= ?)`,
			[]string{"won", "lost"}, "won", since, "lost", since).
		Select("stage, COUNT(*) AS cnt, COALESCE(SUM(amount_cents),0) AS cents").
		Group("stage").Scan(&frows).Error; err != nil {
		return nil, err
	}
	byStage := make(map[string]funnelRow, len(frows))
	for _, r := range frows {
		byStage[r.Stage] = r
	}
	for _, st := range model.Stages {
		r := byStage[st]
		b := StageBucket{Stage: st, Count: r.Cnt, AmountCents: r.Cents}
		out.Funnel = append(out.Funnel, b)
		if st == "won" {
			out.WonCount, out.WonAmountCents = b.Count, b.AmountCents
		}
		if st != "won" && st != "lost" {
			out.OpenOpportunity += b.Count
		}
	}
	tenantQ(s.db.Model(&model.Lead{}), tenantID).Where("created_at >= ?", since).Count(&out.NewLeads)
	tenantQ(s.db.Model(&model.Lead{}), tenantID).Where("status = ? AND updated_at >= ?", "converted", since).Count(&out.ConvertedLeads)
	tenantQ(s.db.Model(&model.Customer{}), tenantID).Count(&out.CustomerCount)
	tenantQ(s.db.Model(&model.Contract{}), tenantID).Where("status = ? AND created_at >= ?", "active", since).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&out.ContractCents)
	// 回款进账只计已确认（审批中/驳回不计，BPM M2 口径）
	tenantQ(s.db.Model(&model.Payment{}), tenantID).
		Where("paid_at >= ? AND status = ?", since, PaymentConfirmed).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&out.PaidCents)
	tenantQ(s.db.Model(&model.FollowUp{}), tenantID).
		Where("next_at IS NOT NULL AND next_at < ?", time.Now()).Count(&out.FollowupsOverdue)
	out.Owners = s.ownerStats(ctx, tenantID, since)
	return out, nil
}

// monthlyTrend returns monthly leads/won/revenue for the last N months.
func (s *Store) monthlyTrend(tenantID uint64, since time.Time) []TrendBucket {
	var buckets []TrendBucket

	type agg struct {
		Month string
		N     int64
		Amt   int64
	}
	var rows []agg

	// leads per month
	if err := tenantQ(s.db.Model(&model.Lead{}), tenantID).
		Where("created_at >= ?", since).
		Select("to_char(created_at, 'YYYY-MM') AS month, COUNT(*) AS n, 0 AS amt").
		Group("month").Order("month").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			buckets = append(buckets, TrendBucket{Month: r.Month, Leads: r.N})
		}
	}
	// won opps per month
	rows = nil
	if err := tenantQ(s.db.Model(&model.Opportunity{}), tenantID).
		Where("stage = ? AND won_at >= ?", "won", since).
		Select("to_char(won_at, 'YYYY-MM') AS month, COUNT(*) AS n, COALESCE(SUM(amount_cents),0) AS amt").
		Group("month").Order("month").Scan(&rows).Error; err == nil {
		for _, r := range rows {
			found := false
			for i := range buckets {
				if buckets[i].Month == r.Month {
					buckets[i].Won = r.N
					buckets[i].Revenue = r.Amt
					found = true
					break
				}
			}
			if !found {
				buckets = append(buckets, TrendBucket{Month: r.Month, Won: r.N, Revenue: r.Amt})
			}
		}
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Month < buckets[j].Month })
	return buckets
}

// TimelineEvent 客户时间线事件。
type TimelineEvent struct {
	Type       string    `json:"type"` // followup | stage_change | contract | lead_convert
	Content    string    `json:"content"`
	ByUserName string    `json:"by_user_name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CustomerTimeline returns all events for a customer in chronological order.
func (s *Store) CustomerTimeline(ctx context.Context, tenantID, customerID uint64) ([]TimelineEvent, error) {
	var events []TimelineEvent

	// follow-ups
	type fu struct {
		Content   string
		ByUserID  uint64
		CreatedAt time.Time
	}
	var fus []fu
	if err := tenantQ(s.db.Model(&model.FollowUp{}), tenantID).
		Where("target_type = ? AND target_id = ?", "customer", customerID).
		Order("created_at DESC").Limit(50).
		Scan(&fus).Error; err == nil {
		for _, f := range fus {
			events = append(events, TimelineEvent{
				Type: "followup", Content: f.Content,
				ByUserName: s.userName(ctx, f.ByUserID), CreatedAt: f.CreatedAt,
			})
		}
	}

	// opportunity stage changes
	type sl struct {
		FromStage, ToStage string
		CreatedAt          time.Time
	}
	var logs []sl
	if err := s.db.Table("crm_opportunity_stage_logs AS l").
		Joins("JOIN crm_opportunities o ON o.id = l.opportunity_id").
		Where("o.tenant_id = ? AND o.customer_id = ?", tenantID, customerID).
		Select("l.from_stage, l.to_stage, l.created_at").
		Order("l.created_at DESC").Limit(20).Scan(&logs).Error; err == nil {
		for _, l := range logs {
			events = append(events, TimelineEvent{
				Type:      "stage_change",
				Content:   fmt.Sprintf("%s → %s", l.FromStage, l.ToStage),
				CreatedAt: l.CreatedAt,
			})
		}
	}

	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.After(events[j].CreatedAt) })
	if len(events) > 50 {
		events = events[:50]
	}
	return events, nil
}

func (s *Store) userName(ctx context.Context, userID uint64) string {
	return s.displayNames(ctx, []uint64{userID})[userID]
}
