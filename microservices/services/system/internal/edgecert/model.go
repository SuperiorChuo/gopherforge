package edgecert

import "time"

// Status 证书生命周期。
const (
	StatusDraft   = "draft"
	StatusPending = "pending"
	StatusIssued  = "issued"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

// Certificate 边缘域名证书行（对应 public.edge_tls_certificates）。
type Certificate struct {
	ID            uint64     `json:"id" gorm:"primaryKey"`
	Domain        string     `json:"domain" gorm:"size:253;not null;uniqueIndex"`
	Email         string     `json:"email" gorm:"size:255;not null"`
	Status        string     `json:"status" gorm:"size:32;not null;default:draft"`
	Provider      string     `json:"provider" gorm:"size:32;not null;default:letsencrypt"`
	IsStaging     bool       `json:"is_staging" gorm:"not null;default:false"`
	FullchainPEM  string     `json:"-" gorm:"column:fullchain_pem;type:text"`
	PrivateKeyPEM string     `json:"-" gorm:"column:private_key_pem;type:text"`
	AccountKeyPEM string     `json:"-" gorm:"column:account_key_pem;type:text"`
	NotBefore     *time.Time `json:"not_before,omitempty"`
	NotAfter      *time.Time `json:"not_after,omitempty"`
	LastError     string     `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (Certificate) TableName() string { return "edge_tls_certificates" }

// PublicView 列表/详情不返回私钥；详情可带 has_cert 标记。
type PublicView struct {
	ID        uint64     `json:"id"`
	Domain    string     `json:"domain"`
	Email     string     `json:"email"`
	Status    string     `json:"status"`
	Provider  string     `json:"provider"`
	IsStaging bool       `json:"is_staging"`
	NotBefore *time.Time `json:"not_before,omitempty"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	HasCert   bool       `json:"has_cert"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (c Certificate) ToPublic() PublicView {
	return PublicView{
		ID: c.ID, Domain: c.Domain, Email: c.Email, Status: c.Status,
		Provider: c.Provider, IsStaging: c.IsStaging,
		NotBefore: c.NotBefore, NotAfter: c.NotAfter, LastError: c.LastError,
		HasCert: c.FullchainPEM != "" && c.PrivateKeyPEM != "",
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
