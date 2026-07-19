package model

import "time"

// ConsoleRoute stores the dynamic web-console route registry migrated from Python.
type ConsoleRoute struct {
	RouteKey        string         `gorm:"column:route_key;size:64;primaryKey" json:"route_key"`
	Path            string         `gorm:"size:255;not null;uniqueIndex" json:"path"`
	Name            string         `gorm:"size:128;not null;uniqueIndex" json:"name"`
	ComponentKey    string         `gorm:"size:128;not null" json:"component_key"`
	Redirect        string         `gorm:"size:255;default:''" json:"redirect"`
	ParentKey       string         `gorm:"size:64;default:'';index" json:"parent_key"`
	SortOrder       int            `gorm:"default:1000;index" json:"sort_order"`
	Hidden          bool           `gorm:"default:false" json:"hidden"`
	Public          bool           `gorm:"default:false" json:"public"`
	Enabled         bool           `gorm:"default:true;index" json:"enabled"`
	PermissionsJSON []string       `gorm:"column:permissions_json;type:json;serializer:json" json:"permissions"`
	RolesJSON       []string       `gorm:"column:roles_json;type:json;serializer:json" json:"roles"`
	MetaJSON        map[string]any `gorm:"column:meta_json;type:json;serializer:json" json:"meta"`
	CreatedAt       time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (ConsoleRoute) TableName() string {
	return "console_routes"
}
