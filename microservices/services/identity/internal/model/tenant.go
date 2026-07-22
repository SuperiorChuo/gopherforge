package model

import "time"

// Tenant is a SaaS tenant boundary (shared-DB row isolation).
type Tenant struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Status    int8      `gorm:"default:1" json:"status"`
	Plan      string    `gorm:"size:64;default:free" json:"plan"`
	MaxUsers  int64     `gorm:"default:0" json:"max_users"`
	// PackageID 绑定的租户套餐（权限包）；NULL = 不限（租户内角色可分配任意权限）。
	PackageID *uint     `gorm:"index" json:"package_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Tenant) TableName() string { return "tenants" }
