package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// Agent binds a Go Admin Kit user to a SIP extension（设计文档 §6.4 身份映射：
// user_id ↔ extension）。话单归因与来电弹屏都依赖这张表。
type Agent struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"not null;default:1;uniqueIndex:ux_cc_agents_tenant_user;uniqueIndex:ux_cc_agents_tenant_ext" json:"tenant_id"`
	UserID      uint64    `gorm:"not null;uniqueIndex:ux_cc_agents_tenant_user" json:"user_id"`
	Extension   string    `gorm:"size:32;not null;uniqueIndex:ux_cc_agents_tenant_ext" json:"extension"`
	DisplayName string    `gorm:"size:64" json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Agent) TableName() string { return "cc_agents" }

// ErrExtensionTaken 表示分机已绑定到同租户的另一个用户。
var ErrExtensionTaken = errors.New("extension already bound to another user")

func (s *Store) ListAgents(ctx context.Context, tenantID uint64) ([]Agent, error) {
	return fullread.All[Agent](s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("extension ASC"))
}

// BindAgent 为 user_id 建立或更新分机绑定（按 tenant+user 幂等 upsert）。
// 分机被同租户其他用户占用时返回 ErrExtensionTaken。
func (s *Store) BindAgent(ctx context.Context, a *Agent) (*Agent, error) {
	a.TenantID = EnsureTenant(a.TenantID)
	a.Extension = strings.TrimSpace(a.Extension)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner Agent
		err := tx.Where("tenant_id = ? AND extension = ?", a.TenantID, a.Extension).First(&owner).Error
		switch {
		case err == nil && owner.UserID != a.UserID:
			return ErrExtensionTaken
		case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		var existing Agent
		err = tx.Where("tenant_id = ? AND user_id = ?", a.TenantID, a.UserID).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(a).Error
		case err != nil:
			return err
		}
		return tx.Model(&Agent{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"extension":    a.Extension,
			"display_name": a.DisplayName,
			"updated_at":   time.Now(),
		}).Error
	})
	if err != nil {
		return nil, err
	}

	var out Agent
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", a.TenantID, a.UserID).
		First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// UnbindAgent 删除绑定；返回是否真的删掉了一行。
// 先查主键再按 id 删——audittrail 插件要求删除谓词含主键。
func (s *Store) UnbindAgent(ctx context.Context, tenantID, userID uint64) (bool, error) {
	var existing Agent
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", EnsureTenant(tenantID), userID).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	res := s.db.WithContext(ctx).
		Where("id = ?", existing.ID).
		Delete(&Agent{})
	return res.RowsAffected > 0, res.Error
}

// GetAgentByExtension 供 webhook 话单归因：agent_ext → 绑定用户。
func (s *Store) GetAgentByExtension(ctx context.Context, tenantID uint64, ext string) (*Agent, error) {
	var a Agent
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND extension = ?", EnsureTenant(tenantID), strings.TrimSpace(ext)).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAgentByExtensionAny 跨租户按分机号查绑定（CDR 租户归属用）。
// 分机号在全局唯一时返回唯一行；未命中返回 (nil, nil)。
func (s *Store) GetAgentByExtensionAny(ctx context.Context, ext string) (*Agent, error) {
	var a Agent
	err := s.db.WithContext(ctx).
		Where("extension = ?", strings.TrimSpace(ext)).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
