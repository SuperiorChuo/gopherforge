package model

import "time"

// OAuth2 client types.
const (
	OAuth2ClientConfidential = 1 // server-side app, authenticates with client_secret
	OAuth2ClientPublic       = 2 // SPA/mobile, PKCE mandatory, no usable secret
)

// OAuth2 grant types (M1 scope).
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
	GrantClientCredentials = "client_credentials"
)

// OAuth2Client is a registered third-party application (the "authorization
// server" side). client_id is globally unique because the token endpoint has
// no tenant context and must resolve the tenant from the client itself.
type OAuth2Client struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TenantID         uint      `gorm:"not null;default:1;index" json:"tenant_id"`
	ClientID         string    `gorm:"size:64;not null;uniqueIndex" json:"client_id"`
	ClientSecretHash string    `gorm:"size:255;not null;default:''" json:"-"`
	Name             string    `gorm:"size:128;not null" json:"name"`
	Logo             string    `gorm:"size:255;not null;default:''" json:"logo"`
	Description      string    `gorm:"size:512;not null;default:''" json:"description"`
	ClientType       int8      `gorm:"not null;default:1" json:"client_type"`
	RedirectURIs     []string  `gorm:"type:jsonb;serializer:json" json:"redirect_uris"`
	Scopes           []string  `gorm:"type:jsonb;serializer:json" json:"scopes"`
	GrantTypes       []string  `gorm:"type:jsonb;serializer:json" json:"grant_types"`
	AccessTokenTTL   int       `gorm:"not null;default:3600" json:"access_token_ttl"`
	RefreshTokenTTL  int       `gorm:"not null;default:2592000" json:"refresh_token_ttl"`
	AutoApprove      bool      `gorm:"not null;default:false" json:"auto_approve"`
	Status           int8      `gorm:"not null;default:1" json:"status"`
	CreatedBy        uint      `gorm:"not null;default:0" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (OAuth2Client) TableName() string { return "oauth2_clients" }

// OAuth2AccessToken is an opaque access token. Only the SHA-256 hash of the
// token string is persisted, so a database leak cannot yield usable tokens.
type OAuth2AccessToken struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TenantID       uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	TokenHash      string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ClientID       string     `gorm:"size:64;not null;index" json:"client_id"`
	UserID         *uint      `gorm:"index" json:"user_id,omitempty"`
	Username       string     `gorm:"size:128;not null;default:''" json:"username"`
	Scopes         []string   `gorm:"type:jsonb;serializer:json" json:"scopes"`
	GrantType      string     `gorm:"size:32;not null;default:''" json:"grant_type"`
	RefreshTokenID *uint      `gorm:"index" json:"refresh_token_id,omitempty"`
	ExpiresAt      time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (OAuth2AccessToken) TableName() string { return "oauth2_access_tokens" }

// OAuth2RefreshToken is an opaque refresh token (SHA-256 hash persisted).
type OAuth2RefreshToken struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	TenantID  uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	TokenHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ClientID  string     `gorm:"size:64;not null;index" json:"client_id"`
	UserID    *uint      `gorm:"index" json:"user_id,omitempty"`
	Username  string     `gorm:"size:128;not null;default:''" json:"username"`
	Scopes    []string   `gorm:"type:jsonb;serializer:json" json:"scopes"`
	GrantType string     `gorm:"size:32;not null;default:''" json:"grant_type"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (OAuth2RefreshToken) TableName() string { return "oauth2_refresh_tokens" }

// OAuth2Approval remembers a user's consent to a client's scopes so repeat
// authorizations skip the consent screen until expiry.
type OAuth2Approval struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"not null;default:1;index" json:"tenant_id"`
	UserID    uint      `gorm:"not null;uniqueIndex:idx_oauth2_approvals_user_client,priority:1" json:"user_id"`
	ClientID  string    `gorm:"size:64;not null;uniqueIndex:idx_oauth2_approvals_user_client,priority:2" json:"client_id"`
	Scopes    []string  `gorm:"type:jsonb;serializer:json" json:"scopes"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (OAuth2Approval) TableName() string { return "oauth2_approvals" }
