package dao

import (
	"context"
	"errors"

	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TagDAO struct{ db *gorm.DB }

func NewTagDAO(db *gorm.DB) *TagDAO { return &TagDAO{db: db} }

func (d *TagDAO) List(ctx context.Context, tenantID uint64) ([]model.Tag, error) {
	var tags []model.Tag
	//nolint:unbounded-find -- 既有接口返回当前租户的完整标签选项，客户表单和标签页均依赖此契约。
	err := d.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("id ASC").Find(&tags).Error
	return tags, err
}

func (d *TagDAO) Create(ctx context.Context, tag *model.Tag) error {
	return d.db.WithContext(ctx).Create(tag).Error
}

func (d *TagDAO) Update(ctx context.Context, tag *model.Tag) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 审计只接受当前租户可见的主键；锁定后再写，避免检查与变更之间出现并发删除。
		var current model.Tag
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", tag.ID, tag.TenantID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 保留无匹配记录时的无操作语义，不把越租户目标送入审计变更链。
		}
		if err != nil {
			return err
		}
		// 保留 GORM 对入参 UpdatedAt 的回填，handler 返回值须与持久化时间一致。
		return tx.Model(tag).Where("tenant_id = ?", tag.TenantID).Updates(map[string]any{
			"name": tag.Name, "color": tag.Color,
		}).Error
	})
}

func (d *TagDAO) Delete(ctx context.Context, id, tenantID uint64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 与更新相同，先在事务内锁定本租户目标，再由原审计回调记录真实删除。
		var current model.Tag
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", id, tenantID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Where("tenant_id = ?", tenantID).Delete(&current).Error
	})
}
