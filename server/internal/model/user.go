package model

import "time"

// User stores account identity and profile fields.
type User struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	Username           string    `gorm:"size:50;not null;uniqueIndex" json:"username"`
	Password           string    `gorm:"size:255;not null" json:"-"`
	Nickname           string    `gorm:"size:50" json:"nickname"`
	Email              string    `gorm:"size:100;uniqueIndex" json:"email"`
	Phone              string    `gorm:"size:20;uniqueIndex" json:"phone"`
	Avatar             string    `gorm:"size:255" json:"avatar"`
	DepartmentID       uint      `gorm:"default:0;index" json:"department_id"`
	MustChangePassword bool      `gorm:"default:false" json:"must_change_password"`
	Status             int8      `gorm:"default:1" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Roles              []Role    `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

// UserRole links users to roles.
type UserRole struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index" json:"user_id"`
	RoleID uint `gorm:"not null;index" json:"role_id"`
}

// OAuthBinding stores third-party account bindings.
type OAuthBinding struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	Provider       string    `gorm:"size:50;not null" json:"provider"`
	ProviderUserID string    `gorm:"size:100;not null" json:"provider_user_id"`
	AccessToken    string    `gorm:"size:255" json:"access_token"`
	RefreshToken   string    `gorm:"size:255" json:"refresh_token"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (OAuthBinding) TableName() string {
	return "oauth_bindings"
}
