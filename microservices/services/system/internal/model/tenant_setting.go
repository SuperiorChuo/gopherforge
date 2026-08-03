package model

import "time"

// TenantSetting 是租户级配置覆盖行：key 命中时租户上下文请求用它，否则回落
// system_settings 平台默认。可配键白名单见 system 服务的 tenantConfigurableKeys。
type TenantSetting struct {
	TenantID   uint           `gorm:"column:tenant_id;primaryKey" json:"tenant_id"`
	SettingKey string         `gorm:"column:setting_key;size:128;primaryKey" json:"setting_key"`
	ValueJSON  map[string]any `gorm:"column:value_json;type:json;serializer:json" json:"value_json"`
	UpdatedAt  time.Time      `gorm:"index" json:"updated_at"`
}

func (TenantSetting) TableName() string {
	return "tenant_settings"
}
