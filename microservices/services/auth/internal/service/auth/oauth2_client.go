package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"

	authdao "github.com/go-admin-kit/services/auth/internal/dao/auth"
	"github.com/go-admin-kit/services/auth/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrOAuth2ClientNotFound = errors.New("oauth2 client not found")
	// ErrOAuth2ClientValidation wraps a human-readable client validation message.
	ErrOAuth2ClientValidation = errors.New("oauth2 client validation failed")
)

// OAuth2ClientValidationError is a typed validation failure carrying the message.
type OAuth2ClientValidationError struct{ Message string }

func (e OAuth2ClientValidationError) Error() string { return e.Message }

// OAuth2ClientService owns application (client) CRUD for the management UI.
type OAuth2ClientService struct {
	clients   authdao.OAuth2ClientDAO
	tokens    authdao.OAuth2TokenDAO
	approvals authdao.OAuth2ApprovalDAO
}

func NewOAuth2ClientServiceWithDB(db *gorm.DB) OAuth2ClientService {
	return OAuth2ClientService{
		clients:   authdao.NewOAuth2ClientDAO(db),
		tokens:    authdao.NewOAuth2TokenDAO(db),
		approvals: authdao.NewOAuth2ApprovalDAO(db),
	}
}

// supported scope/grant catalog (M1 static list).
var (
	supportedScopes = []string{"openid", "profile", "email"}
	supportedGrants = []string{model.GrantAuthorizationCode, model.GrantRefreshToken, model.GrantClientCredentials}
)

// ClientMutation carries the create/update fields from the management API.
type ClientMutation struct {
	Name            string
	Logo            string
	Description     string
	ClientType      int8
	RedirectURIs    []string
	Scopes          []string
	GrantTypes      []string
	AccessTokenTTL  int
	RefreshTokenTTL int
	AutoApprove     bool
	Status          *int8
}

// CreateResult returns the created client plus the one-time plaintext secret.
type CreateResult struct {
	Client       *model.OAuth2Client `json:"client"`
	ClientSecret string              `json:"client_secret"` // shown once; confidential clients only
}

func (s OAuth2ClientService) validate(m ClientMutation) error {
	if strings.TrimSpace(m.Name) == "" {
		return OAuth2ClientValidationError{"应用名称不能为空"}
	}
	if m.ClientType != model.OAuth2ClientConfidential && m.ClientType != model.OAuth2ClientPublic {
		return OAuth2ClientValidationError{"客户端类型无效"}
	}
	if len(m.RedirectURIs) == 0 {
		return OAuth2ClientValidationError{"至少需要一个回调地址"}
	}
	for _, uri := range m.RedirectURIs {
		u, err := url.Parse(uri)
		if err != nil || !u.IsAbs() || u.Fragment != "" {
			return OAuth2ClientValidationError{"回调地址必须是不含 fragment 的绝对 URL：" + uri}
		}
		// Whitelist schemes: only http/https (plus common mobile custom schemes
		// via reverse-DNS form). This rejects javascript:/data: which pass the
		// IsAbs()+no-fragment check yet enable redirect-based XSS on the client.
		if !isAllowedRedirectScheme(u.Scheme) {
			return OAuth2ClientValidationError{"回调地址协议不被允许（仅支持 http/https 或形如 com.example.app 的移动端 scheme）：" + uri}
		}
	}
	for _, sc := range m.Scopes {
		if !containsStr(supportedScopes, sc) {
			return OAuth2ClientValidationError{"不支持的 scope：" + sc}
		}
	}
	if len(m.GrantTypes) == 0 {
		return OAuth2ClientValidationError{"至少选择一种授权模式"}
	}
	for _, gt := range m.GrantTypes {
		if !containsStr(supportedGrants, gt) {
			return OAuth2ClientValidationError{"不支持的授权模式：" + gt}
		}
	}
	// Public clients cannot use client_credentials (no secret to authenticate).
	if m.ClientType == model.OAuth2ClientPublic && containsStr(m.GrantTypes, model.GrantClientCredentials) {
		return OAuth2ClientValidationError{"公开客户端不能使用 client_credentials 模式"}
	}
	return nil
}

// isAllowedRedirectScheme permits http/https (web) and reverse-DNS custom
// schemes (mobile apps, e.g. com.example.app://cb), while rejecting dangerous
// schemes like javascript: and data: that enable redirect-based XSS.
func isAllowedRedirectScheme(scheme string) bool {
	switch scheme {
	case "http", "https":
		return true
	}
	// Mobile custom scheme: reverse-DNS with at least one dot, no colon/space.
	return strings.Contains(scheme, ".") && !strings.ContainsAny(scheme, ": /")
}

func normalizeTTLs(m *ClientMutation) {
	if m.AccessTokenTTL <= 0 {
		m.AccessTokenTTL = 3600
	}
	if m.RefreshTokenTTL <= 0 {
		m.RefreshTokenTTL = 2592000
	}
}

// Create registers a new client. For confidential clients a random secret is
// generated and returned once in plaintext; only its bcrypt hash is stored.
func (s OAuth2ClientService) Create(ctx context.Context, tenantID, createdBy uint, m ClientMutation) (*CreateResult, error) {
	if err := s.validate(m); err != nil {
		return nil, err
	}
	normalizeTTLs(&m)
	clientID, err := randomBase64URL(18)
	if err != nil {
		return nil, err
	}
	client := &model.OAuth2Client{
		TenantID:        tenantID,
		ClientID:        "gak_" + clientID,
		Name:            m.Name,
		Logo:            m.Logo,
		Description:     m.Description,
		ClientType:      m.ClientType,
		RedirectURIs:    m.RedirectURIs,
		Scopes:          m.Scopes,
		GrantTypes:      m.GrantTypes,
		AccessTokenTTL:  m.AccessTokenTTL,
		RefreshTokenTTL: m.RefreshTokenTTL,
		AutoApprove:     m.AutoApprove,
		Status:          1,
		CreatedBy:       createdBy,
	}
	result := &CreateResult{Client: client}
	if m.ClientType == model.OAuth2ClientConfidential {
		secret, err := randomBase64URL(32)
		if err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		client.ClientSecretHash = string(hash)
		result.ClientSecret = secret
	}
	if err := s.clients.CreateContext(ctx, client); err != nil {
		return nil, err
	}
	return result, nil
}

// Update edits an existing client. Changing scopes/redirect_uris invalidates
// remembered approvals so users re-consent under the new terms.
func (s OAuth2ClientService) Update(ctx context.Context, id uint, m ClientMutation) (*model.OAuth2Client, error) {
	if err := s.validate(m); err != nil {
		return nil, err
	}
	normalizeTTLs(&m)
	client, err := s.clients.GetByIDContext(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOAuth2ClientNotFound
	}
	if err != nil {
		return nil, err
	}
	scopesChanged := strings.Join(client.Scopes, " ") != strings.Join(m.Scopes, " ")
	client.Name = m.Name
	client.Logo = m.Logo
	client.Description = m.Description
	client.ClientType = m.ClientType
	client.RedirectURIs = m.RedirectURIs
	client.Scopes = m.Scopes
	client.GrantTypes = m.GrantTypes
	client.AccessTokenTTL = m.AccessTokenTTL
	client.RefreshTokenTTL = m.RefreshTokenTTL
	client.AutoApprove = m.AutoApprove
	if m.Status != nil {
		client.Status = *m.Status
	}
	if err := s.clients.UpdateContext(ctx, client); err != nil {
		return nil, err
	}
	if scopesChanged {
		_ = s.approvals.DeleteByClientContext(ctx, client.ClientID)
	}
	// Disabling a client kills all its live tokens immediately.
	if client.Status != 1 {
		_ = s.tokens.RevokeAllByClientContext(ctx, client.ClientID)
	}
	return client, nil
}

// ResetSecret generates a new secret (confidential clients only), returns it
// once, and revokes all existing tokens + approvals for the client.
func (s OAuth2ClientService) ResetSecret(ctx context.Context, id uint) (string, *model.OAuth2Client, error) {
	client, err := s.clients.GetByIDContext(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, ErrOAuth2ClientNotFound
	}
	if err != nil {
		return "", nil, err
	}
	if client.ClientType != model.OAuth2ClientConfidential {
		return "", nil, OAuth2ClientValidationError{"公开客户端没有密钥"}
	}
	secret, err := randomBase64URL(32)
	if err != nil {
		return "", nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}
	client.ClientSecretHash = string(hash)
	if err := s.clients.UpdateContext(ctx, client); err != nil {
		return "", nil, err
	}
	_ = s.tokens.RevokeAllByClientContext(ctx, client.ClientID)
	_ = s.approvals.DeleteByClientContext(ctx, client.ClientID)
	return secret, client, nil
}

func (s OAuth2ClientService) Get(ctx context.Context, id uint) (*model.OAuth2Client, error) {
	client, err := s.clients.GetByIDContext(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOAuth2ClientNotFound
	}
	return client, err
}

func (s OAuth2ClientService) List(ctx context.Context, keyword string, page, pageSize int) ([]model.OAuth2Client, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.clients.ListContext(ctx, keyword, page, pageSize)
}

// Delete removes a client and cascades to its tokens + approvals.
func (s OAuth2ClientService) Delete(ctx context.Context, id uint) error {
	client, err := s.clients.GetByIDContext(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrOAuth2ClientNotFound
	}
	if err != nil {
		return err
	}
	_ = s.tokens.RevokeAllByClientContext(ctx, client.ClientID)
	_ = s.approvals.DeleteByClientContext(ctx, client.ClientID)
	if _, err := s.clients.DeleteContext(ctx, id); err != nil {
		return err
	}
	return nil
}

// ListTokens returns tenant-scoped access tokens for the management view.
func (s OAuth2ClientService) ListTokens(ctx context.Context, clientID string, page, pageSize int) ([]model.OAuth2AccessToken, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.tokens.ListAccessContext(ctx, clientID, page, pageSize)
}

// RevokeToken revokes one access token by id (management action).
func (s OAuth2ClientService) RevokeToken(ctx context.Context, id uint) error {
	affected, err := s.tokens.RevokeAccessByIDContext(ctx, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOAuth2ClientNotFound
	}
	return nil
}

// SupportedCatalog exposes the static scope/grant options for the UI.
func (s OAuth2ClientService) SupportedCatalog() (scopes, grants []string) {
	return supportedScopes, supportedGrants
}
