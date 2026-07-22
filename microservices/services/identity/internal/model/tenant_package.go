package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// StringList 以 JSON 数组形式落库的字符串集合（Postgres jsonb）。
type StringList []string

// Value 实现 driver.Valuer：序列化为 JSON 数组文本。
func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner：从 jsonb/text 反序列化。
func (s *StringList) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			*s = nil
			return nil
		}
		return json.Unmarshal(v, s)
	case string:
		if v == "" {
			*s = nil
			return nil
		}
		return json.Unmarshal([]byte(v), s)
	default:
		return errors.New("unsupported type for StringList")
	}
}

// GormDataType 声明列类型（迁移里实际用 jsonb）。
func (StringList) GormDataType() string { return "jsonb" }

// TenantPackage 租户套餐（权限包）：permission_codes 圈定租户内角色可分配的权限码集合。
// 平台级目录表，无 tenant_id，只有平台管理员可维护。
type TenantPackage struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:128;not null;uniqueIndex" json:"name"`
	PermissionCodes StringList `gorm:"type:jsonb;not null" json:"permission_codes"`
	Status          int8       `gorm:"default:1" json:"status"`
	Remark          string     `gorm:"size:255;default:''" json:"remark"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (TenantPackage) TableName() string { return "tenant_packages" }
