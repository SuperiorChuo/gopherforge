package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes M2：技能市场的 DAO。挂在现有 *HermesDAO 上（同类型分文件）。

// ErrHermesSkillDuplicate 对应 uk_hermes_skills_tenant_name_version 唯一索引：
// 同一 (tenant, name, version) 只能录入一次，避免同技能名同版本指向不同包。
var ErrHermesSkillDuplicate = errors.New("hermes skill already exists")

// ErrHermesClusterSkillDuplicate 对应 uk_hermes_cluster_skills_cluster_skill：
// 同一集群不能重复绑定同一技能。
var ErrHermesClusterSkillDuplicate = errors.New("cluster skill binding already exists")

func (d *HermesDAO) CreateSkill(ctx context.Context, skill *localmodel.HermesSkill) error {
	err := d.db.WithContext(ctx).Create(skill).Error
	if err != nil && isSkillUniqueViolation(err) {
		return ErrHermesSkillDuplicate
	}
	return err
}

// isSkillUniqueViolation 匹配技能表唯一索引（Postgres 索引名 / 通用措辞）。
func isSkillUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_hermes_skills_tenant_name_version") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

// ListSkills 分页列出技能（content 是 bytea 不进列表查询，避免拖大结果集）。
func (d *HermesDAO) ListSkills(ctx context.Context, page, pageSize int) ([]localmodel.HermesSkill, int64, error) {
	skills := make([]localmodel.HermesSkill, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Omit("content").Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&skills).Error
	return skills, total, err
}

func (d *HermesDAO) GetSkill(ctx context.Context, id uint64) (*localmodel.HermesSkill, error) {
	var skill localmodel.HermesSkill
	err := d.db.WithContext(ctx).Omit("content").First(&skill, id).Error
	return &skill, err
}

// GetSkillContent 取技能 tarball 本体，仅已审批且租户匹配时返回（下载端点用）。
func (d *HermesDAO) GetSkillContent(ctx context.Context, id uint64) ([]byte, error) {
	var content []byte
	err := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Select("content").
		Where("id = ? AND status = ?", id, localmodel.HermesSkillApproved).
		Scan(&content).Error
	return content, err
}

// SetSkillStatus 带来源状态守卫的技能状态流转。
func (d *HermesDAO) SetSkillStatus(ctx context.Context, id uint64, from, to string, actor uint, now time.Time) error {
	fields := map[string]any{"status": to, "updated_at": now}
	if to == localmodel.HermesSkillApproved {
		fields["approved_by"] = actor
		fields["approved_at"] = now
	}
	result := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Where("id = ? AND status = ?", id, from).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set Hermes skill status %s->%s: %w", from, to, gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) BindSkillToCluster(ctx context.Context, clusterID, skillID uint64, actor uint, now time.Time) error {
	binding := &localmodel.HermesClusterSkill{
		TenantID:  tenant.FromContextOrDefault(ctx),
		ClusterID: clusterID,
		SkillID:   skillID,
		CreatedBy: actor,
		CreatedAt: now,
	}
	err := d.db.WithContext(ctx).Create(binding).Error
	if err != nil && isClusterSkillUniqueViolation(err) {
		return ErrHermesClusterSkillDuplicate
	}
	return err
}

func isClusterSkillUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_hermes_cluster_skills_cluster_skill") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

func (d *HermesDAO) UnbindSkillFromCluster(ctx context.Context, clusterID, skillID uint64) error {
	return d.db.WithContext(ctx).
		Where("cluster_id = ? AND skill_id = ?", clusterID, skillID).
		Delete(&localmodel.HermesClusterSkill{}).Error
}

// ListClusterSkills 列出集群绑定的技能（含状态，content 不进列表）。
func (d *HermesDAO) ListClusterSkills(ctx context.Context, clusterID uint64) ([]localmodel.HermesSkill, error) {
	skills := make([]localmodel.HermesSkill, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Select("hermes_skills.id, hermes_skills.tenant_id, hermes_skills.name, hermes_skills.display_name, hermes_skills.description, hermes_skills.version, hermes_skills.sha256, hermes_skills.status, hermes_skills.approved_by, hermes_skills.approved_at, hermes_skills.created_at, hermes_skills.updated_at").
		Joins("JOIN hermes_cluster_skills cs ON cs.skill_id = hermes_skills.id").
		Where("cs.cluster_id = ?", clusterID).
		Order("hermes_skills.name ASC").Scan(&skills).Error
	return skills, err
}

// ListApprovedSkillsForCluster 返回集群绑定的已审批技能（buildDesiredState 用）。
func (d *HermesDAO) ListApprovedSkillsForCluster(ctx context.Context, clusterID uint64) ([]localmodel.HermesSkill, error) {
	skills := make([]localmodel.HermesSkill, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Select("hermes_skills.id, hermes_skills.tenant_id, hermes_skills.name, hermes_skills.display_name, hermes_skills.version, hermes_skills.sha256").
		Joins("JOIN hermes_cluster_skills cs ON cs.skill_id = hermes_skills.id").
		Where("cs.cluster_id = ? AND hermes_skills.status = ?", clusterID, localmodel.HermesSkillApproved).
		Order("hermes_skills.name ASC").Scan(&skills).Error
	return skills, err
}

// ---------------------------------------------------------------------------
// 员工自助技能绑定（M6）
// ---------------------------------------------------------------------------

// ErrHermesUserSkillDuplicate 对应 uk_hermes_user_skills_tenant_user_skill：
// 同一员工重复安装同一技能。服务层将其视为幂等成功。
var ErrHermesUserSkillDuplicate = errors.New("user skill binding already exists")

func (d *HermesDAO) BindUserSkill(ctx context.Context, userID uint, skillID uint64, now time.Time) error {
	binding := &localmodel.HermesUserSkill{
		TenantID:  tenant.FromContextOrDefault(ctx),
		UserID:    userID,
		SkillID:   skillID,
		CreatedAt: now,
	}
	err := d.db.WithContext(ctx).Create(binding).Error
	if err != nil && isUserSkillUniqueViolation(err) {
		return ErrHermesUserSkillDuplicate
	}
	return err
}

func isUserSkillUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_hermes_user_skills_tenant_user_skill") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

func (d *HermesDAO) UnbindUserSkill(ctx context.Context, userID uint, skillID uint64) error {
	return d.db.WithContext(ctx).
		Where("user_id = ? AND skill_id = ?", userID, skillID).
		Delete(&localmodel.HermesUserSkill{}).Error
}

// ListUserSkillIDs 返回员工已绑定的技能 ID（目录 installed 标记用）。
func (d *HermesDAO) ListUserSkillIDs(ctx context.Context, userID uint) ([]uint64, error) {
	ids := make([]uint64, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesUserSkill{}).
		Where("user_id = ?", userID).
		Pluck("skill_id", &ids).Error
	return ids, err
}

// ListApprovedSkillsForUser 返回员工绑定的已审批技能（buildDesiredState 用）。
func (d *HermesDAO) ListApprovedSkillsForUser(ctx context.Context, userID uint) ([]localmodel.HermesSkill, error) {
	skills := make([]localmodel.HermesSkill, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Select("hermes_skills.id, hermes_skills.tenant_id, hermes_skills.name, hermes_skills.display_name, hermes_skills.version, hermes_skills.sha256").
		Joins("JOIN hermes_user_skills us ON us.skill_id = hermes_skills.id").
		Where("us.user_id = ? AND hermes_skills.status = ?", userID, localmodel.HermesSkillApproved).
		Order("hermes_skills.name ASC").Scan(&skills).Error
	return skills, err
}

// ListApprovedSkills 是员工目录：仅已审批技能，content 不进列表。
// 管理员 ListSkills（全状态）保持不变，权限也不放宽。
func (d *HermesDAO) ListApprovedSkills(ctx context.Context, page, pageSize int) ([]localmodel.HermesSkill, int64, error) {
	skills := make([]localmodel.HermesSkill, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Where("status = ?", localmodel.HermesSkillApproved)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Omit("content").Order("name ASC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&skills).Error
	return skills, total, err
}

// UpdateSkillSource 回写官方市场导入的 provenance（repo/path/commit）。
func (d *HermesDAO) UpdateSkillSource(ctx context.Context, id uint64, repo, path, commit string) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"source_repo":   repo,
			"source_path":   path,
			"source_commit": commit,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes skill source %d: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateSkillSignatureStatus 回写供应链验签状态（官方市场导入 → not_applicable）。
func (d *HermesDAO) UpdateSkillSignatureStatus(ctx context.Context, id uint64, status string) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesSkill{}).
		Where("id = ?", id).
		Update("signature_status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes skill signature status %d: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}
