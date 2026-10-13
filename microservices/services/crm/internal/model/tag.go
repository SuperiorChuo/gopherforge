package model

import "time"

type Tag struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;uniqueIndex:ux_crm_tag_name,priority:1" json:"tenant_id"`
	Name      string    `gorm:"size:32;not null;uniqueIndex:ux_crm_tag_name,priority:2" json:"name"`
	Color     string    `gorm:"size:7;not null;default:#1677ff" json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// GORM Preload 填充
	Customers []Customer `gorm:"many2many:crm_customer_tags;" json:"-"`
}

func (Tag) TableName() string { return "crm_tags" }
