package system

import (
	"context"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantSettingDAO 读写 tenant_settings（租户级配置覆盖）。显式按 tenant_id
// 过滤；ctx 带租户时租户插件会加同值过滤，双保险不冲突。
type TenantSettingDAO struct {
	db *gorm.DB
}

func NewTenantSettingDAO(db *gorm.DB) *TenantSettingDAO {
	return &TenantSettingDAO{db: db}
}

func (d *TenantSettingDAO) GetByKeyContext(ctx context.Context, tenantID uint, key string) (*model.TenantSetting, error) {
	var setting model.TenantSetting
	result := d.db.WithContext(ctx).
		Where("tenant_id = ? AND setting_key = ?", tenantID, key).
		First(&setting)
	return &setting, result.Error
}

func (d *TenantSettingDAO) UpsertContext(ctx context.Context, setting *model.TenantSetting) error {
	if setting.UpdatedAt.IsZero() {
		setting.UpdatedAt = time.Now()
	}
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "setting_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
		}).
		Create(setting).Error
}

func (d *TenantSettingDAO) DeleteContext(ctx context.Context, tenantID uint, key string) error {
	result := d.db.WithContext(ctx).
		Where("tenant_id = ? AND setting_key = ?", tenantID, key).
		Delete(&model.TenantSetting{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *TenantSettingDAO) ListContext(ctx context.Context, tenantID uint) ([]model.TenantSetting, error) {
	var settings []model.TenantSetting
	err := d.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("setting_key ASC").
		Find(&settings).Error
	return settings, err
}
