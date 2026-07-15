package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/file/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingDAO struct {
	db *gorm.DB
}

func NewSettingDAO(db *gorm.DB) *SettingDAO {
	return &SettingDAO{db: db}
}

func (d *SettingDAO) ListContext(ctx context.Context, group string) ([]model.SystemSetting, error) {
	var settings []model.SystemSetting
	query := d.dbWithContext(ctx).Model(&model.SystemSetting{})
	if group != "" {
		query = query.Where("setting_key LIKE ?", group+".%")
	}
	err := query.Order("setting_key ASC").Find(&settings).Error
	return settings, err
}

func (d *SettingDAO) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	var setting model.SystemSetting
	result := d.dbWithContext(ctx).Where("setting_key = ?", key).First(&setting)
	return &setting, result.Error
}

func (d *SettingDAO) UpsertContext(ctx context.Context, setting *model.SystemSetting) error {
	if setting.UpdatedAt.IsZero() {
		setting.UpdatedAt = time.Now()
	}
	return d.dbWithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "setting_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
		}).
		Create(setting).Error
}

func (d *SettingDAO) BatchUpsertContext(ctx context.Context, settings []model.SystemSetting) error {
	if len(settings) == 0 {
		return nil
	}
	now := time.Now()
	for i := range settings {
		if settings[i].UpdatedAt.IsZero() {
			settings[i].UpdatedAt = now
		}
	}
	return d.dbWithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "setting_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
		}).
		Create(&settings).Error
}

func (d *SettingDAO) DeleteContext(ctx context.Context, key string) error {
	result := d.dbWithContext(ctx).Where("setting_key = ?", key).Delete(&model.SystemSetting{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *SettingDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}
