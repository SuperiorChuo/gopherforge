package store

import (
	"context"
	"errors"
	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

func (s *Store) ListSkillGroups(tenantID uint64) ([]model.SkillGroup, error) {
	var list []model.SkillGroup
	q := s.db.Order("id ASC")
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Limit(adminListLimit).Find(&list).Error
	return list, err
}

func (s *Store) GetSkillGroup(id uint64) (*model.SkillGroup, error) {
	var g model.SkillGroup
	if err := s.db.First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// GetSkillGroupForTenant 仅在技能组属于 tenantID 时返回。
func (s *Store) GetSkillGroupForTenant(id, tenantID uint64) (*model.SkillGroup, error) {
	var g model.SkillGroup
	if err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) GetSkillGroupByCode(tenantID uint64, code string) (*model.SkillGroup, error) {
	var g model.SkillGroup
	if err := s.db.Where("tenant_id = ? AND code = ? AND status = 1", tenantID, code).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

type SkillGroupInput struct {
	TenantID uint64
	Name     string
	Code     string
	Strategy string
	Status   int16
}

func (s *Store) CreateSkillGroup(ctx context.Context, in SkillGroupInput) (*model.SkillGroup, error) {
	if in.Name == "" || in.Code == "" {
		return nil, errors.New("name and code required")
	}
	if in.Strategy == "" {
		in.Strategy = "round_robin"
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.TenantID == 0 {
		in.TenantID = 1
	}
	// 租户内 code 唯一
	var n int64
	if err := s.db.WithContext(ctx).Model(&model.SkillGroup{}).
		Where("tenant_id = ? AND code = ?", in.TenantID, in.Code).
		Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, errors.New("skill group code already exists in tenant")
	}
	g := model.SkillGroup{
		TenantID: in.TenantID,
		Name:     in.Name,
		Code:     in.Code,
		Strategy: in.Strategy,
		Status:   in.Status,
	}
	if err := s.db.WithContext(ctx).Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) UpdateSkillGroup(ctx context.Context, id, tenantID uint64, in SkillGroupInput) (*model.SkillGroup, error) {
	g, err := s.GetSkillGroupForTenant(id, tenantID)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		g.Name = in.Name
	}
	if in.Code != "" && in.Code != g.Code {
		var n int64
		if err := s.db.WithContext(ctx).Model(&model.SkillGroup{}).
			Where("tenant_id = ? AND code = ? AND id <> ?", tenantID, in.Code, id).
			Count(&n).Error; err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, errors.New("skill group code already exists in tenant")
		}
		g.Code = in.Code
	}
	if in.Strategy != "" {
		g.Strategy = in.Strategy
	}
	if in.Status != 0 {
		g.Status = in.Status
	}
	if err := s.db.WithContext(ctx).Save(g).Error; err != nil {
		return nil, err
	}
	return g, nil
}

func (s *Store) ListAgentSkills(skillGroupID, agentUserID uint64) ([]model.AgentSkill, error) {
	q := s.db.Model(&model.AgentSkill{})
	if skillGroupID > 0 {
		q = q.Where("skill_group_id = ?", skillGroupID)
	}
	if agentUserID > 0 {
		q = q.Where("agent_user_id = ?", agentUserID)
	}
	var list []model.AgentSkill
	err := q.Order("id ASC").Limit(adminListLimit).Find(&list).Error
	return list, err
}

func (s *Store) UpsertAgentSkill(ctx context.Context, agentUserID, skillGroupID uint64, maxConcurrent int, status int16) (*model.AgentSkill, error) {
	if agentUserID == 0 || skillGroupID == 0 {
		return nil, errors.New("agent_user_id and skill_group_id required")
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	if status == 0 {
		status = 1
	}
	var row model.AgentSkill
	err := s.db.WithContext(ctx).Where("agent_user_id = ? AND skill_group_id = ?", agentUserID, skillGroupID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.AgentSkill{
			AgentUserID:   agentUserID,
			SkillGroupID:  skillGroupID,
			MaxConcurrent: maxConcurrent,
			Status:        status,
		}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
		return &row, nil
	}
	if err != nil {
		return nil, err
	}
	row.MaxConcurrent = maxConcurrent
	row.Status = status
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteAgentSkill 仅当技能绑定所属的技能组属于 tenantID 时才删除；
// 跨租户的 id 会以 gorm.ErrRecordNotFound 的形式暴露。
func (s *Store) DeleteAgentSkill(ctx context.Context, id, tenantID uint64) error {
	var row model.AgentSkill
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return err
	}
	if _, err := s.GetSkillGroupForTenant(row.SkillGroupID, tenantID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&model.AgentSkill{}, row.ID).Error
}

func (s *Store) UpsertPresence(agentUserID uint64, status, displayName string) (*model.AgentPresence, error) {
	if agentUserID == 0 {
		return nil, errors.New("agent_user_id required")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "online", "busy", "offline":
	default:
		return nil, errors.New("status must be online|busy|offline")
	}
	now := time.Now()
	var p model.AgentPresence
	err := s.db.First(&p, agentUserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		p = model.AgentPresence{
			AgentUserID: agentUserID,
			Status:      status,
			DisplayName: displayName,
			LastSeenAt:  now,
		}
		if err := s.db.Create(&p).Error; err != nil {
			return nil, err
		}
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	p.Status = status
	p.LastSeenAt = now
	if displayName != "" {
		p.DisplayName = displayName
	}
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPresence(agentUserID uint64) (*model.AgentPresence, error) {
	var p model.AgentPresence
	if err := s.db.First(&p, agentUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.AgentPresence{
				AgentUserID: agentUserID,
				Status:      "offline",
			}, nil
		}
		return nil, err
	}
	return &p, nil
}

// presenceListLimit 兜住坐席在线状态的单次读取规模（坐席台/看板都只展示一屏）。
const presenceListLimit = 500

// ListPresence 列出坐席在线状态。
//
// tenantID > 0 时按租户收口：im_agent_presence 没有 tenant 列，租户归属只能经
// 「坐席 → im_agent_skills → im_skill_groups.tenant_id」这条链判定，因此用
// EXISTS 子查询过滤，只返回在本租户技能组里有绑定的坐席。tenantID == 0 仅供
// 内部全平台任务使用（调用方需自证无租户上下文）。无论如何都带上限，避免把
// 全平台坐席一次性拉进内存。
func (s *Store) ListPresence(tenantID uint64, statuses ...string) ([]model.AgentPresence, error) {
	q := s.db.Model(&model.AgentPresence{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	if tenantID > 0 {
		q = q.Where(`EXISTS (SELECT 1 FROM im_agent_skills sk
			JOIN im_skill_groups sg ON sg.id = sk.skill_group_id
			WHERE sk.agent_user_id = im_agent_presence.agent_user_id AND sg.tenant_id = ?)`, tenantID)
	}
	var list []model.AgentPresence
	err := q.Order("agent_user_id ASC").Limit(presenceListLimit).Find(&list).Error
	return list, err
}

func (s *Store) CountAssignedForAgent(agentUserID uint64) (int64, error) {
	var n int64
	err := s.db.Model(&model.Conversation{}).
		Where("agent_user_id = ? AND status = ?", agentUserID, "assigned").
		Count(&n).Error
	return n, err
}

// CountAssignedForAgents 批量版 CountAssignedForAgent：一条 GROUP BY 顶掉每坐席
// 一次 COUNT。返回 map 对零负载坐席缺键，调用方按 0 读即可（与逐条查询同义）。
func (s *Store) CountAssignedForAgents(agentUserIDs []uint64) (map[uint64]int64, error) {
	out := make(map[uint64]int64, len(agentUserIDs))
	ids := dedupIDs(agentUserIDs)
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		AgentUserID uint64
		N           int64
	}
	var rows []row
	if err := s.db.Model(&model.Conversation{}).
		Select("agent_user_id, COUNT(*) AS n").
		Where("agent_user_id IN ? AND status = ?", ids, "assigned").
		Group("agent_user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.AgentUserID] = r.N
	}
	return out, nil
}

// GetPresenceMap 批量版 GetPresence：一条 IN 查询顶掉每坐席一次 First。
// 与 GetPresence 语义一致——没有 presence 行的坐席补 offline 占位。
func (s *Store) GetPresenceMap(agentUserIDs []uint64) (map[uint64]*model.AgentPresence, error) {
	out := make(map[uint64]*model.AgentPresence, len(agentUserIDs))
	ids := dedupIDs(agentUserIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var list []model.AgentPresence
	//nolint:unbounded-find -- 结果集由调用方提供并去重后的有限坐席 ID 集合约束。
	if err := s.db.Where("agent_user_id IN ?", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		out[list[i].AgentUserID] = &list[i]
	}
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = &model.AgentPresence{AgentUserID: id, Status: "offline"}
		}
	}
	return out, nil
}

// CountAgentSkillsByGroup 批量统计各技能组的坐席绑定数（一条 GROUP BY 顶掉
// 每组一次 ListAgentSkills）。
func (s *Store) CountAgentSkillsByGroup(skillGroupIDs []uint64) (map[uint64]int, error) {
	out := make(map[uint64]int, len(skillGroupIDs))
	ids := dedupIDs(skillGroupIDs)
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		SkillGroupID uint64
		N            int
	}
	var rows []row
	if err := s.db.Model(&model.AgentSkill{}).
		Select("skill_group_id, COUNT(*) AS n").
		Where("skill_group_id IN ?", ids).
		Group("skill_group_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.SkillGroupID] = r.N
	}
	return out, nil
}

// dedupIDs 去重并丢掉 0（IN 里的 0 只会白扫一遍索引）。
func dedupIDs(ids []uint64) []uint64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint64]struct{}, len(ids))
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
