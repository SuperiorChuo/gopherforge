package edgecert

import "time"

const (
	StatusDraft   = "draft"
	StatusPending = "pending"
	StatusIssued  = "issued"
	StatusFailed  = "failed"
	StatusExpired = "expired"

	DeploymentModeExternal    = "external"
	DeploymentModeTraefikFile = "traefik_file"

	DeploymentStatusExternal    = "external"
	DeploymentStatusNotDeployed = "not_deployed"
	DeploymentStatusQueued      = "queued"
	DeploymentStatusRunning     = "running"
	DeploymentStatusInstalled   = "installed"
	DeploymentStatusFailed      = "failed"

	ServingStatusUnchecked   = "unchecked"
	ServingStatusHealthy     = "healthy"
	ServingStatusMismatch    = "mismatch"
	ServingStatusUnreachable = "unreachable"
	ServingStatusInvalid     = "invalid"

	TaskKindIssue  = "issue"
	TaskKindRenew  = "renew"
	TaskKindDeploy = "deploy"
	TaskKindProbe  = "probe"

	TaskStatusQueued    = "queued"
	TaskStatusRunning   = "running"
	TaskStatusSucceeded = "succeeded"
	TaskStatusFailed    = "failed"

	EnvironmentStaging    = "staging"
	EnvironmentProduction = "production"
)

// Certificate 对应 public.edge_tls_certificates。旧明文列仅用于启动迁移，正常写入必须为空。
type Certificate struct {
	ID                        uint64     `json:"id" gorm:"primaryKey"`
	Domain                    string     `json:"domain" gorm:"size:253;not null;uniqueIndex"`
	Email                     string     `json:"email" gorm:"size:255;not null"`
	Status                    string     `json:"status" gorm:"size:32;not null;default:draft"`
	Provider                  string     `json:"provider" gorm:"size:32;not null;default:letsencrypt"`
	IsStaging                 bool       `json:"is_staging" gorm:"not null;default:false"`
	FullchainPEM              string     `json:"-" gorm:"column:fullchain_pem;type:text"`
	PrivateKeyPEM             string     `json:"-" gorm:"column:private_key_pem;type:text"`
	AccountKeyPEM             string     `json:"-" gorm:"column:account_key_pem;type:text"`
	PrivateKeyEnc             string     `json:"-" gorm:"column:private_key_enc;type:text"`
	AccountKeyEnc             string     `json:"-" gorm:"column:account_key_enc;type:text"`
	CertFingerprintSHA256     string     `json:"cert_fingerprint_sha256,omitempty" gorm:"column:cert_fingerprint_sha256;size:64"`
	DeploymentMode            string     `json:"deployment_mode" gorm:"size:32;not null;default:external"`
	DeploymentProvider        string     `json:"deployment_provider,omitempty" gorm:"size:64"`
	AutoRenewEnabled          bool       `json:"auto_renew_enabled" gorm:"not null;default:false"`
	RenewAt                   *time.Time `json:"renew_at,omitempty"`
	LastRenewalAt             *time.Time `json:"last_renewal_at,omitempty"`
	DeploymentStatus          string     `json:"deployment_status" gorm:"size:32;not null;default:external"`
	DeployedFingerprintSHA256 string     `json:"deployed_fingerprint_sha256,omitempty" gorm:"column:deployed_fingerprint_sha256;size:64"`
	DeployedAt                *time.Time `json:"deployed_at,omitempty"`
	ServingStatus             string     `json:"serving_status" gorm:"size:32;not null;default:unchecked"`
	ServingFingerprintSHA256  string     `json:"serving_fingerprint_sha256,omitempty" gorm:"column:serving_fingerprint_sha256;size:64"`
	ServingNotAfter           *time.Time `json:"serving_not_after,omitempty"`
	ServingIssuer             string     `json:"serving_issuer,omitempty" gorm:"type:text"`
	ServingCheckedAt          *time.Time `json:"serving_checked_at,omitempty"`
	ServingErrorCode          string     `json:"serving_error_code,omitempty" gorm:"size:64"`
	ServingErrorMessage       string     `json:"serving_error_message,omitempty" gorm:"type:text"`
	NotBefore                 *time.Time `json:"not_before,omitempty"`
	NotAfter                  *time.Time `json:"not_after,omitempty"`
	LastError                 string     `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

func (Certificate) TableName() string { return "edge_tls_certificates" }

// Task 是可租约接管的持久化任务。
type Task struct {
	ID                 uint64     `json:"id" gorm:"primaryKey"`
	CertificateID      uint64     `json:"certificate_id" gorm:"not null;index"`
	Kind               string     `json:"kind" gorm:"size:16;not null"`
	Status             string     `json:"status" gorm:"size:16;not null;default:queued"`
	Step               string     `json:"step" gorm:"size:32;not null;default:queued"`
	Environment        string     `json:"environment" gorm:"size:16;not null"`
	RequestedBy        uint64     `json:"requested_by" gorm:"not null;default:0"`
	AttemptCount       int        `json:"attempt_count" gorm:"not null;default:0"`
	RunAfter           time.Time  `json:"run_after" gorm:"not null;index"`
	LeaseOwner         string     `json:"-" gorm:"size:64"`
	LeaseUntil         *time.Time `json:"-"`
	ProviderOrderURI   string     `json:"-" gorm:"column:provider_order_uri;type:text"`
	ProviderCertKeyEnc string     `json:"-" gorm:"column:provider_cert_key_enc;type:text"`
	ErrorCode          string     `json:"error_code,omitempty" gorm:"size:64"`
	ErrorMessage       string     `json:"error_message,omitempty" gorm:"type:text"`
	ErrorHint          string     `json:"error_hint,omitempty" gorm:"type:text"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
}

func (Task) TableName() string { return "edge_cert_tasks" }

// Challenge 供任意 system 副本响应 HTTP-01。
type Challenge struct {
	Token            string    `gorm:"primaryKey;size:255"`
	CertificateID    uint64    `gorm:"not null;index"`
	KeyAuthorization string    `gorm:"type:text;not null"`
	ExpiresAt        time.Time `gorm:"not null;index"`
	CreatedAt        time.Time `gorm:"not null"`
}

func (Challenge) TableName() string { return "edge_acme_challenges" }

type PublicView struct {
	ID                        uint64           `json:"id"`
	Domain                    string           `json:"domain"`
	Email                     string           `json:"email"`
	Status                    string           `json:"status"`
	Provider                  string           `json:"provider"`
	IsStaging                 bool             `json:"is_staging"`
	NotBefore                 *time.Time       `json:"not_before,omitempty"`
	NotAfter                  *time.Time       `json:"not_after,omitempty"`
	RenewAt                   *time.Time       `json:"renew_at,omitempty"`
	LastRenewalAt             *time.Time       `json:"last_renewal_at,omitempty"`
	LastError                 string           `json:"last_error,omitempty"`
	HasCert                   bool             `json:"has_cert"`
	CertFingerprintSHA256     string           `json:"cert_fingerprint_sha256,omitempty"`
	DeploymentMode            string           `json:"deployment_mode"`
	DeploymentProvider        string           `json:"deployment_provider,omitempty"`
	AutoRenewEnabled          bool             `json:"auto_renew_enabled"`
	DeploymentStatus          string           `json:"deployment_status"`
	DeployedFingerprintSHA256 string           `json:"deployed_fingerprint_sha256,omitempty"`
	DeployedAt                *time.Time       `json:"deployed_at,omitempty"`
	ServingStatus             string           `json:"serving_status"`
	ServingFingerprintSHA256  string           `json:"serving_fingerprint_sha256,omitempty"`
	ServingNotAfter           *time.Time       `json:"serving_not_after,omitempty"`
	ServingIssuer             string           `json:"serving_issuer,omitempty"`
	ServingCheckedAt          *time.Time       `json:"serving_checked_at,omitempty"`
	ServingErrorCode          string           `json:"serving_error_code,omitempty"`
	ServingErrorMessage       string           `json:"serving_error_message,omitempty"`
	CreatedAt                 time.Time        `json:"created_at"`
	UpdatedAt                 time.Time        `json:"updated_at"`
	ActiveTask                *Task            `json:"active_task,omitempty"`
	Certificate               CertificateState `json:"certificate"`
	Issuance                  IssuanceState    `json:"issuance"`
	Deployment                DeploymentState  `json:"deployment"`
	Renewal                   RenewalState     `json:"renewal"`
	Serving                   ServingState     `json:"serving"`
}

type CertificateState struct {
	Status            string     `json:"status"`
	HasCertificate    bool       `json:"has_certificate"`
	FingerprintSHA256 string     `json:"fingerprint_sha256,omitempty"`
	NotBefore         *time.Time `json:"not_before,omitempty"`
	NotAfter          *time.Time `json:"not_after,omitempty"`
}

type IssuanceState struct {
	Status      string `json:"status"`
	Environment string `json:"environment"`
	Provider    string `json:"provider"`
	LastError   string `json:"last_error,omitempty"`
}

type DeploymentState struct {
	Mode                      string     `json:"mode"`
	Provider                  string     `json:"provider,omitempty"`
	Status                    string     `json:"status"`
	DeployedFingerprintSHA256 string     `json:"deployed_fingerprint_sha256,omitempty"`
	DeployedAt                *time.Time `json:"deployed_at,omitempty"`
}

type RenewalState struct {
	Status        string     `json:"status"`
	AutoEnabled   bool       `json:"auto_enabled"`
	RenewAt       *time.Time `json:"renew_at,omitempty"`
	LastRenewalAt *time.Time `json:"last_renewal_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type ServingState struct {
	Status                  string     `json:"status"`
	ManagedCertificateInUse bool       `json:"managed_certificate_in_use"`
	FingerprintSHA256       string     `json:"fingerprint_sha256,omitempty"`
	NotAfter                *time.Time `json:"not_after,omitempty"`
	Issuer                  string     `json:"issuer,omitempty"`
	CheckedAt               *time.Time `json:"checked_at,omitempty"`
	ErrorCode               string     `json:"error_code,omitempty"`
	ErrorMessage            string     `json:"error_message,omitempty"`
}

func (c Certificate) ToPublic() PublicView {
	return c.toPublicAt(time.Now().UTC())
}

func (c Certificate) toPublicAt(now time.Time) PublicView {
	hasCert := c.FullchainPEM != "" && c.PrivateKeyEnc != ""
	view := PublicView{
		ID: c.ID, Domain: c.Domain, Email: c.Email, Status: c.Status,
		Provider: c.Provider, IsStaging: c.IsStaging, NotBefore: c.NotBefore,
		NotAfter: c.NotAfter, RenewAt: c.RenewAt, LastRenewalAt: c.LastRenewalAt,
		LastError: c.LastError, HasCert: hasCert,
		CertFingerprintSHA256: c.CertFingerprintSHA256,
		DeploymentMode:        c.DeploymentMode, DeploymentProvider: c.DeploymentProvider,
		AutoRenewEnabled: c.AutoRenewEnabled, DeploymentStatus: c.DeploymentStatus,
		DeployedFingerprintSHA256: c.DeployedFingerprintSHA256, DeployedAt: c.DeployedAt,
		ServingStatus: c.ServingStatus, ServingFingerprintSHA256: c.ServingFingerprintSHA256,
		ServingNotAfter: c.ServingNotAfter, ServingIssuer: c.ServingIssuer,
		ServingCheckedAt: c.ServingCheckedAt, ServingErrorCode: c.ServingErrorCode,
		ServingErrorMessage: c.ServingErrorMessage, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	view.Certificate = CertificateState{Status: certificateState(c, now), HasCertificate: hasCert, FingerprintSHA256: c.CertFingerprintSHA256, NotBefore: c.NotBefore, NotAfter: c.NotAfter}
	view.Issuance = IssuanceState{Status: "idle", Environment: environmentFor(c), Provider: c.Provider, LastError: c.LastError}
	view.Deployment = DeploymentState{Mode: c.DeploymentMode, Provider: c.DeploymentProvider, Status: c.DeploymentStatus, DeployedFingerprintSHA256: c.DeployedFingerprintSHA256, DeployedAt: c.DeployedAt}
	view.Renewal = RenewalState{Status: renewalState(c, now), AutoEnabled: c.AutoRenewEnabled, RenewAt: c.RenewAt, LastRenewalAt: c.LastRenewalAt, LastError: c.LastError}
	view.Serving = ServingState{Status: c.ServingStatus, ManagedCertificateInUse: c.DeploymentMode == DeploymentModeTraefikFile && c.ServingStatus == ServingStatusHealthy && c.ServingFingerprintSHA256 == c.CertFingerprintSHA256, FingerprintSHA256: c.ServingFingerprintSHA256, NotAfter: c.ServingNotAfter, Issuer: c.ServingIssuer, CheckedAt: c.ServingCheckedAt, ErrorCode: c.ServingErrorCode, ErrorMessage: c.ServingErrorMessage}
	return view
}

func certificateState(c Certificate, now time.Time) string {
	if c.FullchainPEM == "" || c.PrivateKeyEnc == "" || c.NotAfter == nil {
		return "none"
	}
	if !c.NotAfter.After(now) {
		return "expired"
	}
	if (c.RenewAt != nil && !c.RenewAt.After(now)) || c.NotAfter.Before(now.Add(30*24*time.Hour)) {
		return "expiring"
	}
	return "valid"
}

func renewalState(c Certificate, now time.Time) string {
	if !c.AutoRenewEnabled {
		return "disabled"
	}
	if c.RenewAt == nil {
		return "awaiting_certificate"
	}
	if !c.RenewAt.After(now) {
		return "due"
	}
	return "scheduled"
}

type Capabilities struct {
	EncryptionConfigured bool     `json:"encryption_configured"`
	AsyncTasks           bool     `json:"async_tasks"`
	DurableHTTP01        bool     `json:"durable_http01"`
	DeploymentModes      []string `json:"deployment_modes"`
	AutomaticRenewal     bool     `json:"automatic_renewal"`
}
