package store

import (
	"errors"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sort"
	"time"
)

// CreateConversation 在 botEnabled 时以 bot_serving 状态创建，否则进入排队（+ 自动分配）。
// tenantID 继承自站点；0 会被归一化为 1。
func (s *Store) CreateConversation(tenantID, siteID, visitorID uint64, channel, contextJSON string, skillGroupID *uint64, botEnabled bool) (*model.Conversation, error) {
	now := time.Now()
	status := "queued"
	var queuedAt *time.Time
	if botEnabled {
		status = "bot_serving"
	} else {
		queuedAt = &now
	}
	if tenantID == 0 {
		tenantID = 1
	}
	c := model.Conversation{
		PublicID:     uuid.New(),
		TenantID:     tenantID,
		SiteID:       siteID,
		Channel:      channel,
		VisitorID:    visitorID,
		SkillGroupID: skillGroupID,
		Status:       status,
		Context:      contextJSON,
		QueuedAt:     queuedAt,
	}
	if c.Channel == "" {
		c.Channel = "h5"
	}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	if status == "queued" {
		_ = s.TryAutoAssign(&c)
	} else {
		_ = s.AppendSystemEvent(c.ID, "bot_started", map[string]any{"status": "bot_serving"})
	}
	return &c, nil
}

func (s *Store) GetConversationByPublicID(publicID string) (*model.Conversation, error) {
	id, err := uuid.Parse(publicID)
	if err != nil {
		return nil, err
	}
	var c model.Conversation
	if err := s.db.Where("public_id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListAgentConversations 范围：all | mine | queue | bot。始终按 tenantID 过滤。
func (s *Store) ListAgentConversations(tenantID, agentUserID uint64, scope string, skillGroupID uint64, limit int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	if tenantID == 0 {
		tenantID = 1
	}
	q := s.db.Model(&model.Conversation{}).Where("tenant_id = ?", tenantID)
	switch scope {
	case "mine":
		q = q.Where("status = ? AND agent_user_id = ?", "assigned", agentUserID)
	case "queue":
		q = q.Where("status = ?", "queued")
		if skillGroupID > 0 {
			q = q.Where("skill_group_id = ?", skillGroupID)
		}
	case "bot":
		q = q.Where("status = ?", "bot_serving")
	case "closed": // 历史会话（含评分），最近关闭在前
		q = q.Where("status = ?", "closed")
		if skillGroupID > 0 {
			q = q.Where("skill_group_id = ?", skillGroupID)
		}
		var list []model.Conversation
		err := q.Order("closed_at DESC NULLS LAST").Limit(limit).Find(&list).Error
		return list, err
	default: // all open
		q = q.Where("status IN ?", []string{"queued", "assigned", "bot_serving"})
		if skillGroupID > 0 {
			q = q.Where("(skill_group_id = ? OR skill_group_id IS NULL OR status = 'bot_serving')", skillGroupID)
		}
	}
	var list []model.Conversation
	err := q.Order("last_message_at DESC NULLS LAST, created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// ListQueuedForSkillGroups 一次取多个技能组的排队会话（坐席上线时批量派单用），
// 顶掉「每技能组一次 ListAgentConversations」。排序与 scope=queue 一致。
func (s *Store) ListQueuedForSkillGroups(tenantID uint64, skillGroupIDs []uint64, limit int) ([]model.Conversation, error) {
	ids := dedupIDs(skillGroupIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if tenantID == 0 {
		tenantID = 1
	}
	var list []model.Conversation
	//nolint:unbounded-find -- 查询由调用方上限 limit 和有限技能组 ID 集合共同约束。
	err := s.db.Where("tenant_id = ? AND status = ? AND skill_group_id IN ?", tenantID, "queued", ids).
		Order("last_message_at DESC NULLS LAST, created_at DESC").
		Limit(limit).Find(&list).Error
	return list, err
}

// TransferToHuman 将 bot_serving → queued（然后自动分配）。
func (s *Store) TransferToHuman(c *model.Conversation, reason string) error {
	if c.Status == "closed" {
		return errors.New("conversation already closed")
	}
	if c.Status == "assigned" {
		return errors.New("already with agent")
	}
	now := time.Now()
	c.Status = "queued"
	c.QueuedAt = &now
	c.AgentUserID = nil
	c.AssignedAt = nil
	if err := s.db.Save(c).Error; err != nil {
		return err
	}
	if reason == "" {
		reason = "visitor"
	}
	_ = s.AppendSystemEvent(c.ID, "transfer_human", map[string]any{"reason": reason})
	_ = s.TryAutoAssign(c)
	return nil
}

func (s *Store) SaveSummary(c *model.Conversation, summary string) error {
	c.Summary = summary
	return s.db.Model(c).Update("summary", summary).Error
}

func (s *Store) AssignConversation(c *model.Conversation, agentUserID uint64) error {
	now := time.Now()
	c.AgentUserID = &agentUserID
	c.Status = "assigned"
	c.AssignedAt = &now
	return s.db.Save(c).Error
}

type CloseOpts struct {
	Reason string
}

func (s *Store) CloseConversation(c *model.Conversation, opts CloseOpts) error {
	now := time.Now()
	c.Status = "closed"
	c.ClosedAt = &now
	if opts.Reason != "" {
		c.CloseReason = opts.Reason
	} else if c.CloseReason == "" {
		c.CloseReason = "agent"
	}
	return s.db.Save(c).Error
}

// RateConversation 记录访客 CSAT（M5）：仅限已关闭会话，且只评一次。
func (s *Store) RateConversation(c *model.Conversation, score int16, comment string) error {
	if c.Status != "closed" {
		return errors.New("conversation is not closed")
	}
	if c.RatingScore != nil {
		return errors.New("already rated")
	}
	if score < 1 || score > 5 {
		return errors.New("score must be 1-5")
	}
	now := time.Now()
	c.RatingScore = &score
	c.RatingComment = comment
	c.RatedAt = &now
	return s.db.Model(c).Updates(map[string]any{
		"rating_score":   score,
		"rating_comment": comment,
		"rated_at":       now,
	}).Error
}

// TransferConversation 将会话转给其他坐席，或重新排队。
// targetAgentID == 0 → 重新排队（可选转入新的技能组）。
func (s *Store) TransferConversation(c *model.Conversation, fromAgentID, targetAgentID uint64, skillGroupID *uint64, note string) error {
	if c.Status == "closed" {
		return errors.New("conversation already closed")
	}
	now := time.Now()
	if skillGroupID != nil {
		if *skillGroupID == 0 {
			c.SkillGroupID = nil
		} else {
			c.SkillGroupID = skillGroupID
		}
	}
	if targetAgentID == 0 {
		c.AgentUserID = nil
		c.Status = "queued"
		c.QueuedAt = &now
		c.AssignedAt = nil
		if err := s.db.Save(c).Error; err != nil {
			return err
		}
		_ = s.AppendSystemEvent(c.ID, "transferred_queue", map[string]any{
			"from_agent_user_id": fromAgentID,
			"skill_group_id":     c.SkillGroupID,
			"note":               note,
		})
		// 重新排队后尝试自动分配
		_ = s.TryAutoAssign(c)
		return nil
	}
	c.AgentUserID = &targetAgentID
	c.Status = "assigned"
	c.AssignedAt = &now
	if err := s.db.Save(c).Error; err != nil {
		return err
	}
	_ = s.AppendSystemEvent(c.ID, "transferred", map[string]any{
		"from_agent_user_id": fromAgentID,
		"to_agent_user_id":   targetAgentID,
		"skill_group_id":     c.SkillGroupID,
		"note":               note,
	})
	return nil
}

// TryAutoAssign 按策略在技能组中挑选一个在线坐席。
func (s *Store) TryAutoAssign(c *model.Conversation) error {
	if c == nil || c.Status != "queued" {
		return nil
	}
	sgID := uint64(0)
	if c.SkillGroupID != nil {
		sgID = *c.SkillGroupID
	}
	if sgID == 0 {
		// 无技能组 → 跳过自动分配（仅手动接单）
		return nil
	}
	sg, err := s.GetSkillGroup(sgID)
	if err != nil || sg.Status != 1 {
		return err
	}
	if sg.Strategy == "manual" {
		return nil
	}

	var skills []model.AgentSkill
	//nolint:unbounded-find -- 自动分配必须读取单个技能组的完整启用坐席集合。
	if err := s.db.Where("skill_group_id = ? AND status = 1", sgID).Find(&skills).Error; err != nil {
		return err
	}
	if len(skills) == 0 {
		return nil
	}

	// 过滤未达到 max concurrent 上限的 online/busy 坐席
	type candidate struct {
		UserID uint64
		Load   int64
		Max    int
		Status string
	}
	// 批量取 presence + 在接数：原来每候选坐席 2 次查询（GetPresence +
	// CountAssignedForAgent），派单是每条排队会话都走的热路径。
	agentIDs := make([]uint64, 0, len(skills))
	for _, sk := range skills {
		agentIDs = append(agentIDs, sk.AgentUserID)
	}
	presences, err := s.GetPresenceMap(agentIDs)
	if err != nil {
		return err
	}
	loads, err := s.CountAssignedForAgents(agentIDs)
	if err != nil {
		return err
	}
	var cands []candidate
	for _, sk := range skills {
		p := presences[sk.AgentUserID]
		if p == nil {
			continue
		}
		if p.Status != "online" && p.Status != "busy" {
			continue
		}
		// busy 仅在策略需要且仍在负载内时才接收？自动分配跳过 busy
		if p.Status == "busy" {
			continue
		}
		load := loads[sk.AgentUserID]
		if int(load) >= sk.MaxConcurrent {
			continue
		}
		cands = append(cands, candidate{UserID: sk.AgentUserID, Load: load, Max: sk.MaxConcurrent, Status: p.Status})
	}
	if len(cands) == 0 {
		return nil
	}

	var pick uint64
	switch sg.Strategy {
	case "least_load":
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].Load == cands[j].Load {
				return cands[i].UserID < cands[j].UserID
			}
			return cands[i].Load < cands[j].Load
		})
		pick = cands[0].UserID
	default: // round_robin
		sort.Slice(cands, func(i, j int) bool { return cands[i].UserID < cands[j].UserID })
		// 选取 RRCursor 之后的下一个
		pick = cands[0].UserID
		for i, ca := range cands {
			if ca.UserID > sg.RRCursor {
				pick = ca.UserID
				// 从该索引开始轮转
				_ = i
				break
			}
		}
		// 如果全部都 <= cursor，则回绕到第一个
		if pick == cands[0].UserID && cands[len(cands)-1].UserID <= sg.RRCursor {
			pick = cands[0].UserID
		}
		// 更新 cursor
		_ = s.db.Model(sg).Update("rr_cursor", pick).Error
	}

	if err := s.AssignConversation(c, pick); err != nil {
		return err
	}
	_ = s.AppendSystemEvent(c.ID, "assigned", map[string]any{
		"agent_user_id":  pick,
		"skill_group_id": sgID,
		"strategy":       sg.Strategy,
		"auto":           true,
	})
	return nil
}

func (s *Store) AppendSystemEvent(conversationID uint64, event string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["event"] = event
	content := JSONText(payload)
	msg := &model.Message{
		ConversationID: conversationID,
		SenderType:     "system",
		MsgType:        "event",
		Content:        content,
	}
	return s.CreateMessage(msg)
}

func (s *Store) EnsureOpenConversation(tenantID, siteID, visitorID uint64, channel, contextJSON string, skillGroupID *uint64, botEnabled bool) (*model.Conversation, error) {
	var c model.Conversation
	err := s.db.Where("visitor_id = ? AND status IN ?", visitorID, []string{"queued", "assigned", "created", "bot_serving"}).
		Order("id DESC").First(&c).Error
	if err == nil {
		// 复用未关闭的会话，但刷新 context，让坐席能看到最新的页面 URL / IP。
		if contextJSON != "" && contextJSON != "{}" && contextJSON != "null" {
			_ = s.UpdateConversationContext(c.ID, contextJSON)
			c.Context = contextJSON
		}
		return &c, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return s.CreateConversation(tenantID, siteID, visitorID, channel, contextJSON, skillGroupID, botEnabled)
}

// ListVisitorConversations 返回访客最近会话（任意状态），
// 最新在前——支撑坐席端历史面板。
func (s *Store) ListVisitorConversations(visitorID uint64, limit int) ([]model.Conversation, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var list []model.Conversation
	err := s.db.Where("visitor_id = ?", visitorID).
		Order("created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}
