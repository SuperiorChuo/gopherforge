package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
)

// Hermes M6：员工自助升级。身份永远来自控制台会话（handler requireUserID），
// 端点不接受请求体里的 user_id；员工只能绑定已审批技能（roadmap 安全基线）。

// SelfBindSkill 员工自助安装：只能装已审批技能；重复安装幂等成功。
func (s *HermesService) SelfBindSkill(ctx context.Context, userID uint, skillID uint64) error {
	if userID == 0 {
		return fmt.Errorf("%w: missing user", ErrHermesInvalidInput)
	}
	skill, err := s.dao.GetSkill(ctx, skillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: skill not found", ErrHermesInvalidInput)
		}
		return err
	}
	// 员工可装已审核技能，不可越权装未审/未授权项（roadmap 安全基线）
	if skill.Status != localmodel.HermesSkillApproved {
		return fmt.Errorf("%w: skill is not approved", ErrHermesInvalidInput)
	}
	err = s.dao.BindUserSkill(ctx, userID, skillID, s.now().UTC())
	if errors.Is(err, dao.ErrHermesUserSkillDuplicate) {
		return nil // 幂等：重复点安装不算错误
	}
	return err
}

// SelfUnbindSkill 员工自助卸载（幂等）。
func (s *HermesService) SelfUnbindSkill(ctx context.Context, userID uint, skillID uint64) error {
	if userID == 0 {
		return fmt.Errorf("%w: missing user", ErrHermesInvalidInput)
	}
	return s.dao.UnbindUserSkill(ctx, userID, skillID)
}

// HermesSkillCatalogItem 是员工技能目录条目：已审批技能 + 本人是否已安装。
type HermesSkillCatalogItem struct {
	localmodel.HermesSkill
	Installed bool `json:"installed"`
}

// ListSkillCatalog 员工技能目录：仅已审批技能，installed 标记折进单次响应。
func (s *HermesService) ListSkillCatalog(ctx context.Context, userID uint, page, pageSize int) ([]HermesSkillCatalogItem, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	skills, total, err := s.dao.ListApprovedSkills(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	installedIDs, err := s.dao.ListUserSkillIDs(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	installed := make(map[uint64]bool, len(installedIDs))
	for _, id := range installedIDs {
		installed[id] = true
	}
	items := make([]HermesSkillCatalogItem, 0, len(skills))
	for _, skill := range skills {
		items = append(items, HermesSkillCatalogItem{HermesSkill: skill, Installed: installed[skill.ID]})
	}
	return items, total, nil
}

// buildDesiredStateUserSkills 载入员工自助绑定的已审批技能（镜像
// buildDesiredStateSkills，同 SourceURL/sha256 形状）。
func (s *HermesService) buildDesiredStateUserSkills(ctx context.Context, userID uint) ([]HermesDesiredStateSkill, error) {
	if userID == 0 {
		return nil, nil
	}
	skills, err := s.dao.ListApprovedSkillsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, nil
	}
	out := make([]HermesDesiredStateSkill, 0, len(skills))
	for _, skill := range skills {
		out = append(out, HermesDesiredStateSkill{
			ID:        skill.ID,
			Name:      skill.Name,
			Version:   skill.Version,
			SHA256:    skill.SHA256,
			SourceURL: fmt.Sprintf("/api/v1/ai/hermes/device/skills/%d/download", skill.ID),
		})
	}
	return out, nil
}

// mergeDesiredStateSkills 合并集群技能与员工自助技能：按 Name 去重（name 是
// profile skills/<name>/ 磁盘目录，同名两版本会撞盘），冲突时**集群优先**
// （管理员意图覆盖自助）。结果按 Name 排序保证签名载荷与 SkillsAppliedDigest
// 稳定；双空返回 nil（omitempty，P1 payload 字节不变）。
func mergeDesiredStateSkills(cluster, user []HermesDesiredStateSkill) []HermesDesiredStateSkill {
	if len(cluster) == 0 && len(user) == 0 {
		return nil
	}
	byName := make(map[string]HermesDesiredStateSkill, len(cluster)+len(user))
	for _, skill := range user {
		byName[skill.Name] = skill
	}
	for _, skill := range cluster {
		byName[skill.Name] = skill // 集群覆盖同名自助绑定
	}
	out := make([]HermesDesiredStateSkill, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
