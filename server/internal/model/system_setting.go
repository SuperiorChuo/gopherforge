package model

import "time"

// SystemSetting stores console and runtime settings migrated from Python.
type SystemSetting struct {
	SettingKey string         `gorm:"column:setting_key;size:128;primaryKey" json:"setting_key"`
	ValueJSON  map[string]any `gorm:"column:value_json;type:json;serializer:json" json:"value_json"`
	UpdatedAt  time.Time      `gorm:"index" json:"updated_at"`
}

func (SystemSetting) TableName() string {
	return "system_settings"
}
