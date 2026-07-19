package model

import "time"

// Notice stores announcements and notifications (tenant-scoped).
type Notice struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	TenantID  uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	Title     string     `gorm:"size:200;not null" json:"title"`
	Content   string     `gorm:"type:text;not null" json:"content"`
	Type      int8       `gorm:"default:1" json:"type"`
	Status    int8       `gorm:"default:1" json:"status"`
	CreatorID uint       `gorm:"index" json:"creator_id"`
	Creator   string     `gorm:"size:50" json:"creator"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Notice) TableName() string {
	return "notices"
}
