package model

import "time"

// Invite 是邀请注册单次使用的邀请行。token 明文只在创建时返回一次（管理员分发
// 链接），库里只存 SHA-256(token)；注册时按 hash 查 + 原子消费（used_at）。
type Invite struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	TenantID  uint       `gorm:"not null;index" json:"tenant_id"`
	RoleID    uint       `gorm:"default:0" json:"role_id,omitempty"` // 0 = 不分配角色
	Email     string     `gorm:"size:255;default:''" json:"email,omitempty"`
	TokenHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedBy uint       `gorm:"default:0" json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Invite) TableName() string { return "invites" }
