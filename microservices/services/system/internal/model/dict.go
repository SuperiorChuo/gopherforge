package model

import "time"

// DictType stores dictionary categories.
type DictType struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:100;not null" json:"name"`
	Code        string     `gorm:"size:100;not null;uniqueIndex" json:"code"`
	Description string     `gorm:"size:255" json:"description"`
	Status      int8       `gorm:"default:1" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Items       []DictItem `gorm:"-" json:"items,omitempty"`
}

// DictItem stores dictionary values.
type DictItem struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DictTypeID uint      `gorm:"not null;index" json:"dict_type_id"`
	Label      string    `gorm:"size:100;not null" json:"label"`
	Value      string    `gorm:"size:100;not null" json:"value"`
	Sort       int       `gorm:"default:0" json:"sort"`
	Status     int8      `gorm:"default:1" json:"status"`
	Remark     string    `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
