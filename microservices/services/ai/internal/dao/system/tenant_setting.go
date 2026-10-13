package system

import (
	"context"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// TenantSettingDAO 读取 tenant_settings（租户级配置覆盖）。ai 服务只读不改。
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
