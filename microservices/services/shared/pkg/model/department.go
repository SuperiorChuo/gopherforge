package model

import "time"

// Department stores organization hierarchy data.
type Department struct {
	ID        uint         `gorm:"primaryKey" json:"id"`
	TenantID  uint         `gorm:"not null;default:1;uniqueIndex:ux_depts_tenant_code,priority:1;index" json:"tenant_id"`
	Name      string       `gorm:"size:100;not null" json:"name"`
	Code      string       `gorm:"size:50;uniqueIndex:ux_depts_tenant_code,priority:2" json:"code"`
	ParentID  uint         `gorm:"default:0;index" json:"parent_id"`
	Leader    string       `gorm:"size:50" json:"leader"`
	Phone     string       `gorm:"size:20" json:"phone"`
	Email     string       `gorm:"size:100" json:"email"`
	Sort      int          `gorm:"default:0" json:"sort"`
	Status    int8         `gorm:"default:1" json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Children  []Department `gorm:"-" json:"children,omitempty"`
}
