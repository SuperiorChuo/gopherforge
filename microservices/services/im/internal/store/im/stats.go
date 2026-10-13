package store

import (
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
	"time"
)

// LowRatedFilter 列出 CSAT 较差的已关闭会话供质检（QC）。
type LowRatedFilter struct {
	// MaxScore 含该值；默认 3（1–3 星）。
	MaxScore int
	// Days 回溯天数；0 = 30。
	Days int
	// AgentUserID 可选过滤条件。
	AgentUserID uint64
	Page        int
	PageSize    int
}

// ListLowRatedConversations 返回已关闭且已评分、分数 ≤ max 的会话。
func (s *Store) ListLowRatedConversations(tenantID uint64, f LowRatedFilter) ([]model.Conversation, int64, error) {
	if tenantID == 0 {
		tenantID = 1
	}
	maxScore := f.MaxScore
	if maxScore <= 0 || maxScore > 5 {
		maxScore = 3
	}
	days := f.Days
	if days <= 0 || days > 365 {
		days = 30
	}
	page, ps := f.Page, f.PageSize
	if page <= 0 {
		page = 1
	}
	if ps <= 0 || ps > 50 {
		ps = 20
	}
	since := time.Now().AddDate(0, 0, -days)
	q := s.db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND status = ? AND rating_score IS NOT NULL AND rating_score <= ? AND closed_at >= ?",
			tenantID, "closed", maxScore, since)
	if f.AgentUserID > 0 {
		q = q.Where("agent_user_id = ?", f.AgentUserID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.Conversation
	err := q.Order("rating_score ASC, closed_at DESC").
		Offset((page - 1) * ps).Limit(ps).Find(&list).Error
	return list, total, err
}

type AgentStat struct {
	AgentUserID uint64   `json:"agent_user_id"`
	DisplayName string   `json:"display_name"`
	Closed      int64    `json:"closed"`
	Rated       int64    `json:"rated"`
	AvgRating   *float64 `json:"avg_rating,omitempty"`
}

type StatsOverview struct {
	TotalOpen     int64       `json:"total_open"`
	TotalQueued   int64       `json:"total_queued"`
	ClosedInRange int64       `json:"closed_in_range"`
	RatedInRange  int64       `json:"rated_in_range"`
	AvgRating     *float64    `json:"avg_rating,omitempty"`
	ScoreDist     map[int]int `json:"score_dist"`
	Agents        []AgentStat `json:"agents"`
}

// Stats 汇总租户在 [since, now] 区间内的工作量与 CSAT。
// 会话量与 ACD 分配走行级查询已够用；报表口径按 closed_at 落在区间内。
func (s *Store) Stats(tenantID uint64, since time.Time) (*StatsOverview, error) {
	if tenantID == 0 {
		tenantID = 1
	}
	out := &StatsOverview{ScoreDist: map[int]int{}}
	base := s.db.Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	if err := base.Session(&gorm.Session{}).Where("status IN ?", []string{"queued", "assigned", "bot_serving"}).Count(&out.TotalOpen).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Where("status = ?", "queued").Count(&out.TotalQueued).Error; err != nil {
		return nil, err
	}
	closed := s.db.Model(&model.Conversation{}).Where("tenant_id = ? AND status = ? AND closed_at >= ?", tenantID, "closed", since)
	if err := closed.Session(&gorm.Session{}).Count(&out.ClosedInRange).Error; err != nil {
		return nil, err
	}
	if err := closed.Session(&gorm.Session{}).Where("rating_score IS NOT NULL").Count(&out.RatedInRange).Error; err != nil {
		return nil, err
	}
	if out.RatedInRange > 0 {
		var avg float64
		if err := closed.Session(&gorm.Session{}).Where("rating_score IS NOT NULL").
			Select("AVG(rating_score)").Scan(&avg).Error; err != nil {
			return nil, err
		}
		out.AvgRating = &avg
		type distRow struct {
			Score int
			N     int
		}
		var dist []distRow
		if err := closed.Session(&gorm.Session{}).Where("rating_score IS NOT NULL").
			Select("rating_score AS score, COUNT(*) AS n").Group("rating_score").Scan(&dist).Error; err != nil {
			return nil, err
		}
		for _, d := range dist {
			out.ScoreDist[d.Score] = d.N
		}
	}
	// 按坐席：区间内的关闭数 + 评分统计
	type agentRow struct {
		AgentUserID uint64
		Closed      int64
		Rated       int64
		AvgRating   *float64
	}
	var rows []agentRow
	if err := s.db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND status = ? AND closed_at >= ? AND agent_user_id IS NOT NULL", tenantID, "closed", since).
		Select("agent_user_id, COUNT(*) AS closed, COUNT(rating_score) AS rated, AVG(rating_score) AS avg_rating").
		Group("agent_user_id").
		Order("closed DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	// 只取报表里真正出现的坐席展示名：原实现会无条件读取全表，
	// 既跨租户读了全平台在线状态，又没有上限。
	names := map[uint64]string{}
	agentIDs := make([]uint64, 0, len(rows))
	for _, r := range rows {
		agentIDs = append(agentIDs, r.AgentUserID)
	}
	if presences, err := s.GetPresenceMap(agentIDs); err == nil {
		for id, p := range presences {
			names[id] = p.DisplayName
		}
	}
	for _, r := range rows {
		out.Agents = append(out.Agents, AgentStat{
			AgentUserID: r.AgentUserID,
			DisplayName: names[r.AgentUserID],
			Closed:      r.Closed,
			Rated:       r.Rated,
			AvgRating:   r.AvgRating,
		})
	}
	return out, nil
}

// QueuePosition 返回 conv 在排队会话中的 1 基位置
// （同租户；按 queued_at 再按 id 的 FIFO）。
func (s *Store) QueuePosition(conv *model.Conversation) int64 {
	if conv.Status != "queued" {
		return 0
	}
	var ahead int64
	q := s.db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND status = ?", conv.TenantID, "queued")
	if conv.QueuedAt != nil {
		q = q.Where("(queued_at < ?) OR (queued_at = ? AND id < ?)", *conv.QueuedAt, *conv.QueuedAt, conv.ID)
	} else {
		q = q.Where("id < ?", conv.ID)
	}
	if err := q.Count(&ahead).Error; err != nil {
		return 0
	}
	return ahead + 1
}

// OnlineAgentCount 报告 online/busy 状态的坐席数，用于离线通知决策。
func (s *Store) OnlineAgentCount() (int64, error) {
	var n int64
	err := s.db.Model(&model.AgentPresence{}).Where("status IN ?", []string{"online", "busy"}).Count(&n).Error
	return n, err
}

// CloseIdleConversations 关闭最后一条消息早于 cutoff 的未关闭非 bot 会话。
// 返回已关闭的会话供事件/广播使用。
func (s *Store) CloseIdleConversations(idle time.Duration, limit int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 100
	}
	cutoff := time.Now().Add(-idle)
	var list []model.Conversation
	// bot_serving 不超时：无人工占用成本；last_message_at 为空按 created_at 算
	//nolint:unbounded-find -- 查询由后台任务传入的 limit 显式约束。
	if err := s.db.Where("status IN ?", []string{"queued", "assigned"}).
		Where("COALESCE(last_message_at, created_at) < ?", cutoff).
		Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	closed := make([]model.Conversation, 0, len(list))
	for i := range list {
		c := &list[i]
		if err := s.CloseConversation(c, CloseOpts{Reason: "timeout"}); err != nil {
			continue
		}
		_ = s.AppendSystemEvent(c.ID, "closed", map[string]any{"reason": "timeout", "auto": true})
		closed = append(closed, *c)
	}
	return closed, nil
}
