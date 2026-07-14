package model

import "time"

// Permission stores API and UI permission metadata.
type Permission struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name"`
	Code        string       `gorm:"size:100;not null;uniqueIndex" json:"code"`
	Description string       `gorm:"size:255;default:''" json:"description"`
	Type        int8         `gorm:"not null" json:"type"`
	Path        string       `gorm:"size:255" json:"path"`
	Method      string       `gorm:"size:10" json:"method"`
	ParentID    uint         `gorm:"default:0" json:"parent_id"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Roles       []Role       `gorm:"many2many:role_permissions;" json:"roles,omitempty"`
	Children    []Permission `gorm:"-" json:"children,omitempty"`
}
