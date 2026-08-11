package model

import "time"

// Menu stores backend-managed frontend menu metadata.
type Menu struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name"`
	Title       string       `gorm:"size:50;not null" json:"title"`
	Icon        string       `gorm:"size:100" json:"icon"`
	Path        string       `gorm:"size:255" json:"path"`
	Component   string       `gorm:"size:255" json:"component"`
	ParentID    uint         `gorm:"default:0;index" json:"parent_id"`
	Sort        int          `gorm:"default:0" json:"sort"`
	Status      int8         `gorm:"default:1" json:"status"`
	Hidden      int8         `gorm:"default:0" json:"hidden"`
	Permission  string       `gorm:"size:100" json:"permission"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Children    []Menu       `gorm:"-" json:"children,omitempty"`
	Permissions []Permission `gorm:"many2many:menu_permissions;" json:"permissions,omitempty"`
}

// MenuPermission links menus to permissions.
type MenuPermission struct {
	ID           uint `gorm:"primaryKey" json:"id"`
	MenuID       uint `gorm:"not null;index" json:"menu_id"`
	PermissionID uint `gorm:"not null;index" json:"permission_id"`
}
