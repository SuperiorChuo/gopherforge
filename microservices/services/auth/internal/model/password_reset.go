package localmodel

import "time"

// PasswordReset 是忘记密码的单次使用重置令牌。与 Invite 同模式：库里只存
// sha256(token)，明文仅经邮件一次性下发；原子消费（used_at）防重放。
type PasswordReset struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	TenantID  uint       `gorm:"not null;default:1" json:"tenant_id"`
	TokenHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (PasswordReset) TableName() string { return "password_resets" }
