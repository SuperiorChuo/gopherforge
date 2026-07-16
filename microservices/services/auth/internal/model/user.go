package model

import "time"

// User stores account identity and profile fields.
type User struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	TenantID           uint       `gorm:"not null;default:1;uniqueIndex:ux_users_tenant_username,priority:1;index" json:"tenant_id"`
	Username           string     `gorm:"size:50;not null;uniqueIndex:ux_users_tenant_username,priority:2" json:"username"`
	Password           string     `gorm:"size:255;not null" json:"-"`
	Nickname           string     `gorm:"size:50" json:"nickname"`
	Email              string     `gorm:"size:100" json:"email"`
	Phone              string     `gorm:"size:20" json:"phone"`
	Avatar             string     `gorm:"size:255" json:"avatar"`
	DepartmentID       uint       `gorm:"default:0;index" json:"department_id"`
	MustChangePassword bool       `gorm:"default:false" json:"must_change_password"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
	TOTPSecret         string     `gorm:"size:255" json:"-"`
	TOTPEnabled        bool       `gorm:"default:false" json:"totp_enabled"`
	Status             int8       `gorm:"default:1" json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Roles              []Role     `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

// PasswordHistory stores previous password hashes for reuse checks.
type PasswordHistory struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"not null;index;index:idx_password_history_user_changed" json:"user_id"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	ChangedAt    time.Time `gorm:"not null;index:idx_password_history_user_changed" json:"changed_at"`
	CreatedAt    time.Time `json:"created_at"`
}

func (PasswordHistory) TableName() string {
	return "password_history"
}

// TOTPRecoveryCode stores hashed one-time recovery codes for two-factor login.
type TOTPRecoveryCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index;index:idx_totp_recovery_codes_user_unused" json:"user_id"`
	CodeHash  string     `gorm:"size:255;not null" json:"-"`
	UsedAt    *time.Time `gorm:"index:idx_totp_recovery_codes_user_unused" json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (TOTPRecoveryCode) TableName() string {
	return "totp_recovery_codes"
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
