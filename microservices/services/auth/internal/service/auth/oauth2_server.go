package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	authdao "github.com/go-admin-kit/services/auth/internal/dao/auth"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// OAuth2Error carries an RFC 6749 error code and the HTTP status the token /
// introspection endpoints should return. It is the transport for every
// protocol-level failure so handlers can emit the bare RFC JSON shape.
type OAuth2Error struct {
	Code        string
	Description string
	Status      int
}

func (e *OAuth2Error) Error() string { return e.Code + ": " + e.Description }

func oauth2Err(status int, code, desc string) *OAuth2Error {
	return &OAuth2Error{Code: code, Description: desc, Status: status}
}

// AuthorizeRequest is the parsed /oauth2/authorize query.
type AuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// AuthorizeView is what the consent screen renders.
type AuthorizeView struct {
	ClientID        string   `json:"client_id"`
	ClientName      string   `json:"client_name"`
	Logo            string   `json:"logo"`
	Description     string   `json:"description"`
	Scopes          []string `json:"scopes"`
	State           string   `json:"state"`
	RedirectURI     string   `json:"redirect_uri"`
	AutoApprove     bool     `json:"auto_approve"`
	AlreadyApproved bool     `json:"already_approved"`
}

// TokenResponse is the RFC 6749 token endpoint success body.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// OAuth2ServerService implements the authorization-server protocol endpoints.
type OAuth2ServerService struct {
	clients   authdao.OAuth2ClientDAO
	tokens    authdao.OAuth2TokenDAO
	approvals authdao.OAuth2ApprovalDAO
	users     *UserService
	cache     *cache.CacheService
}

func NewOAuth2ServerServiceWithDB(db *gorm.DB, redis cache.RedisClient) *OAuth2ServerService {
	users := NewUserServiceWithDB(db)
	var cacheSvc *cache.CacheService
	if redis != nil {
		cacheSvc = cache.NewCacheServiceWithClient(redis)
	} else {
		cacheSvc = cache.NewCacheService()
	}
	return &OAuth2ServerService{
		clients:   authdao.NewOAuth2ClientDAO(db),
		tokens:    authdao.NewOAuth2TokenDAO(db),
		approvals: authdao.NewOAuth2ApprovalDAO(db),
		users:     &users,
		cache:     cacheSvc,
	}
}

const approvalTTL = 180 * 24 * time.Hour // consent remembered for 180 days

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func parseScopes(raw string) []string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func scopesSubset(requested, allowed []string) bool {
	set := make(map[string]bool, len(allowed))
	for _, s := range allowed {
		set[s] = true
	}
	for _, s := range requested {
		if !set[s] {
			return false
		}
	}
	return true
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// loadActiveClient fetches an enabled client by client_id (global, no tenant scope).
func (s *OAuth2ServerService) loadActiveClient(ctx context.Context, clientID string) (*model.OAuth2Client, *OAuth2Error) {
	client, err := s.clients.GetByClientIDContext(ctx, clientID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, oauth2Err(401, "invalid_client", "client not found")
	}
	if err != nil {
		return nil, oauth2Err(500, "server_error", "failed to load client")
	}
	if client.Status != 1 {
		return nil, oauth2Err(401, "invalid_client", "client is disabled")
	}
	return client, nil
}

// tenantCtx builds a context carrying the client's tenant so downstream
// tenant-scoped writes land under the right tenant (token endpoint has no auth ctx).
func tenantCtx(ctx context.Context, tenantID uint) context.Context {
	return context.WithValue(ctx, middleware.TenantIDContextKey, tenantID)
}

// ValidateAuthorizeRequest runs the pre-consent validation chain. Any failure
// returns an OAuth2Error and the caller renders an error page — it never emits
// a redirect built from unvalidated input (open-redirect guard).
func (s *OAuth2ServerService) ValidateAuthorizeRequest(ctx context.Context, req AuthorizeRequest, userID uint) (*AuthorizeView, *OAuth2Error) {
	if req.ClientID == "" {
		return nil, oauth2Err(400, "invalid_request", "client_id is required")
	}
	client, oerr := s.loadActiveClient(ctx, req.ClientID)
	if oerr != nil {
		return nil, oerr
	}
	// redirect_uri must exactly match a registered value.
	if req.RedirectURI == "" || !containsStr(client.RedirectURIs, req.RedirectURI) {
		return nil, oauth2Err(400, "invalid_request", "redirect_uri does not match a registered value")
	}
	if req.ResponseType != "code" {
		return nil, oauth2Err(400, "unsupported_response_type", "only response_type=code is supported")
	}
	if !containsStr(client.GrantTypes, model.GrantAuthorizationCode) {
		return nil, oauth2Err(400, "unauthorized_client", "client may not use authorization_code")
	}
	requested := parseScopes(req.Scope)
	if len(requested) == 0 {
		requested = client.Scopes // default to full registered set
	}
	if !scopesSubset(requested, client.Scopes) {
		return nil, oauth2Err(400, "invalid_scope", "requested scope exceeds client registration")
	}
	// PKCE: public clients MUST use S256; plain is rejected for everyone.
	if req.CodeChallenge != "" && req.CodeChallengeMethod != "S256" {
		return nil, oauth2Err(400, "invalid_request", "only code_challenge_method=S256 is supported")
	}
	if client.ClientType == model.OAuth2ClientPublic && req.CodeChallenge == "" {
		return nil, oauth2Err(400, "invalid_request", "PKCE code_challenge is required for public clients")
	}

	view := &AuthorizeView{
		ClientID:    client.ClientID,
		ClientName:  client.Name,
		Logo:        client.Logo,
		Description: client.Description,
		Scopes:      requested,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		AutoApprove: client.AutoApprove,
	}
	if approval, err := s.approvals.GetContext(ctx, userID, client.ClientID); err == nil && approval != nil {
		if scopesSubset(requested, approval.Scopes) {
			view.AlreadyApproved = true
		}
	}
	return view, nil
}

// Approve re-runs the full validation (never trusts the client-submitted view),
// records consent, mints a single-use code and returns the redirect URL.
func (s *OAuth2ServerService) Approve(ctx context.Context, userID uint, username string, tenantID uint, req AuthorizeRequest) (string, *OAuth2Error) {
	view, oerr := s.ValidateAuthorizeRequest(ctx, req, userID)
	if oerr != nil {
		return "", oerr
	}
	code, err := randomBase64URL(32)
	if err != nil {
		return "", oauth2Err(500, "server_error", "failed to generate code")
	}
	payload := cache.OAuth2CodePayload{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		UserID:              userID,
		Username:            username,
		TenantID:            tenantID,
		Scopes:              view.Scopes,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
	}
	if err := s.cache.StoreOAuth2CodeContext(ctx, code, payload); err != nil {
		return "", oauth2Err(500, "server_error", "failed to persist code")
	}
	if err := s.approvals.UpsertContext(ctx, tenantID, userID, req.ClientID, view.Scopes, time.Now().Add(approvalTTL)); err != nil {
		return "", oauth2Err(500, "server_error", "failed to record approval")
	}
	redirect, err := appendQuery(req.RedirectURI, map[string]string{"code": code, "state": req.State})
	if err != nil {
		return "", oauth2Err(400, "invalid_request", "malformed redirect_uri")
	}
	return redirect, nil
}

// DenyRedirect builds the access_denied redirect when the user rejects consent.
// client_id + redirect_uri are validated first so we never redirect to an
// unregistered destination.
func (s *OAuth2ServerService) DenyRedirect(ctx context.Context, req AuthorizeRequest) (string, *OAuth2Error) {
	client, oerr := s.loadActiveClient(ctx, req.ClientID)
	if oerr != nil {
		return "", oerr
	}
	if req.RedirectURI == "" || !containsStr(client.RedirectURIs, req.RedirectURI) {
		return "", oauth2Err(400, "invalid_request", "redirect_uri does not match a registered value")
	}
	redirect, err := appendQuery(req.RedirectURI, map[string]string{"error": "access_denied", "state": req.State})
	if err != nil {
		return "", oauth2Err(400, "invalid_request", "malformed redirect_uri")
	}
	return redirect, nil
}

func appendQuery(rawURL string, params map[string]string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// AuthenticateClientContext validates client credentials at the token endpoint.
// Confidential clients must present a matching secret; public clients present
// none (authenticated via PKCE at code exchange).
func (s *OAuth2ServerService) AuthenticateClientContext(ctx context.Context, clientID, secret string) (*model.OAuth2Client, *OAuth2Error) {
	client, oerr := s.loadActiveClient(ctx, clientID)
	if oerr != nil {
		return nil, oerr
	}
	if client.ClientType == model.OAuth2ClientConfidential {
		if secret == "" {
			return nil, oauth2Err(401, "invalid_client", "client secret is required")
		}
		if bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(secret)) != nil {
			return nil, oauth2Err(401, "invalid_client", "invalid client secret")
		}
	}
	return client, nil
}

// ExchangeAuthorizationCode handles grant_type=authorization_code.
func (s *OAuth2ServerService) ExchangeAuthorizationCode(ctx context.Context, client *model.OAuth2Client, code, redirectURI, codeVerifier string) (*TokenResponse, *OAuth2Error) {
	if !containsStr(client.GrantTypes, model.GrantAuthorizationCode) {
		return nil, oauth2Err(400, "unauthorized_client", "client may not use authorization_code")
	}
	payload, err := s.cache.ConsumeOAuth2CodeContext(ctx, code)
	if errors.Is(err, cache.ErrOAuth2CodeNotFound) {
		return nil, oauth2Err(400, "invalid_grant", "authorization code is invalid or expired")
	}
	if err != nil {
		return nil, oauth2Err(500, "server_error", "failed to read code")
	}
	// Bind checks: code must belong to this client and redirect_uri.
	if payload.ClientID != client.ClientID {
		return nil, oauth2Err(400, "invalid_grant", "code was issued to another client")
	}
	if payload.RedirectURI != redirectURI {
		return nil, oauth2Err(400, "invalid_grant", "redirect_uri mismatch")
	}
	// PKCE verification.
	if payload.CodeChallenge != "" {
		if codeVerifier == "" {
			return nil, oauth2Err(400, "invalid_grant", "code_verifier is required")
		}
		if subtle.ConstantTimeCompare([]byte(codeChallengeS256(codeVerifier)), []byte(payload.CodeChallenge)) != 1 {
			return nil, oauth2Err(400, "invalid_grant", "PKCE verification failed")
		}
	}
	uid := payload.UserID
	return s.issueTokens(ctx, client, &uid, payload.Username, payload.TenantID, payload.Scopes, model.GrantAuthorizationCode)
}

// ExchangeRefreshToken handles grant_type=refresh_token with rotation: the old
// refresh token and its access token are revoked as new ones are minted.
func (s *OAuth2ServerService) ExchangeRefreshToken(ctx context.Context, client *model.OAuth2Client, refreshToken string) (*TokenResponse, *OAuth2Error) {
	if !containsStr(client.GrantTypes, model.GrantRefreshToken) {
		return nil, oauth2Err(400, "unauthorized_client", "client may not use refresh_token")
	}
	hash := sha256Hex(refreshToken)
	stored, err := s.tokens.GetRefreshByHashContext(ctx, hash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, oauth2Err(400, "invalid_grant", "refresh token is invalid")
	}
	if err != nil {
		return nil, oauth2Err(500, "server_error", "failed to read refresh token")
	}
	if stored.ClientID != client.ClientID {
		return nil, oauth2Err(400, "invalid_grant", "refresh token was issued to another client")
	}
	if stored.RevokedAt != nil || time.Now().After(stored.ExpiresAt) {
		return nil, oauth2Err(400, "invalid_grant", "refresh token is expired or revoked")
	}
	tctx := tenantCtx(ctx, stored.TenantID)
	// Rotate: revoke the old refresh token + its access token.
	if _, err := s.tokens.RevokeRefreshByHashContext(tctx, hash); err != nil {
		return nil, oauth2Err(500, "server_error", "failed to rotate refresh token")
	}
	if err := s.tokens.RevokeAccessByRefreshTokenIDContext(tctx, stored.ID); err != nil {
		return nil, oauth2Err(500, "server_error", "failed to rotate access token")
	}
	return s.issueTokens(ctx, client, stored.UserID, stored.Username, stored.TenantID, stored.Scopes, model.GrantRefreshToken)
}

// ClientCredentials handles grant_type=client_credentials (confidential only,
// no user, no refresh token per RFC recommendation).
func (s *OAuth2ServerService) ClientCredentials(ctx context.Context, client *model.OAuth2Client, scope string) (*TokenResponse, *OAuth2Error) {
	if client.ClientType != model.OAuth2ClientConfidential {
		return nil, oauth2Err(400, "unauthorized_client", "only confidential clients may use client_credentials")
	}
	if !containsStr(client.GrantTypes, model.GrantClientCredentials) {
		return nil, oauth2Err(400, "unauthorized_client", "client may not use client_credentials")
	}
	requested := parseScopes(scope)
	if len(requested) == 0 {
		requested = client.Scopes
	}
	if !scopesSubset(requested, client.Scopes) {
		return nil, oauth2Err(400, "invalid_scope", "requested scope exceeds client registration")
	}
	tctx := tenantCtx(ctx, client.TenantID)
	accessToken, oerr := s.mintAccessToken(tctx, client, nil, "", client.TenantID, requested, model.GrantClientCredentials, nil)
	if oerr != nil {
		return nil, oerr
	}
	return &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   client.AccessTokenTTL,
		Scope:       strings.Join(requested, " "),
	}, nil
}

// issueTokens mints a linked refresh + access token pair (authorization_code /
// refresh_token grants).
func (s *OAuth2ServerService) issueTokens(ctx context.Context, client *model.OAuth2Client, userID *uint, username string, tenantID uint, scopes []string, grantType string) (*TokenResponse, *OAuth2Error) {
	tctx := tenantCtx(ctx, tenantID)
	refreshRaw, err := randomBase64URL(32)
	if err != nil {
		return nil, oauth2Err(500, "server_error", "failed to generate refresh token")
	}
	refresh := &model.OAuth2RefreshToken{
		TenantID:  tenantID,
		TokenHash: sha256Hex(refreshRaw),
		ClientID:  client.ClientID,
		UserID:    userID,
		Username:  username,
		Scopes:    scopes,
		GrantType: grantType,
		ExpiresAt: time.Now().Add(time.Duration(client.RefreshTokenTTL) * time.Second),
	}
	if err := s.tokens.CreateRefreshContext(tctx, refresh); err != nil {
		return nil, oauth2Err(500, "server_error", "failed to persist refresh token")
	}
	accessToken, oerr := s.mintAccessToken(tctx, client, userID, username, tenantID, scopes, grantType, &refresh.ID)
	if oerr != nil {
		return nil, oerr
	}
	return &TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    client.AccessTokenTTL,
		RefreshToken: refreshRaw,
		Scope:        strings.Join(scopes, " "),
	}, nil
}

func (s *OAuth2ServerService) mintAccessToken(ctx context.Context, client *model.OAuth2Client, userID *uint, username string, tenantID uint, scopes []string, grantType string, refreshID *uint) (string, *OAuth2Error) {
	raw, err := randomBase64URL(32)
	if err != nil {
		return "", oauth2Err(500, "server_error", "failed to generate access token")
	}
	access := &model.OAuth2AccessToken{
		TenantID:       tenantID,
		TokenHash:      sha256Hex(raw),
		ClientID:       client.ClientID,
		UserID:         userID,
		Username:       username,
		Scopes:         scopes,
		GrantType:      grantType,
		RefreshTokenID: refreshID,
		ExpiresAt:      time.Now().Add(time.Duration(client.AccessTokenTTL) * time.Second),
	}
	if err := s.tokens.CreateAccessContext(ctx, access); err != nil {
		return "", oauth2Err(500, "server_error", "failed to persist access token")
	}
	return raw, nil
}

// lookupActiveAccessToken resolves a bearer access token to its live DB row.
func (s *OAuth2ServerService) lookupActiveAccessToken(ctx context.Context, rawToken string) (*model.OAuth2AccessToken, error) {
	token, err := s.tokens.GetAccessByHashContext(ctx, sha256Hex(rawToken))
	if err != nil {
		return nil, err
	}
	if token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		return nil, errors.New("token inactive")
	}
	return token, nil
}

// Introspect implements RFC 7662. Always returns a map; inactive tokens report
// {"active": false} without leaking why.
func (s *OAuth2ServerService) Introspect(ctx context.Context, rawToken string) map[string]any {
	token, err := s.lookupActiveAccessToken(ctx, rawToken)
	if err != nil {
		return map[string]any{"active": false}
	}
	result := map[string]any{
		"active":     true,
		"client_id":  token.ClientID,
		"scope":      strings.Join(token.Scopes, " "),
		"token_type": "Bearer",
		"exp":        token.ExpiresAt.Unix(),
		"iat":        token.CreatedAt.Unix(),
	}
	if token.UserID != nil {
		result["sub"] = *token.UserID
		result["username"] = token.Username
	}
	return result
}

// Revoke implements RFC 7009 — always succeeds (idempotent). Only revokes
// tokens belonging to the authenticated client. Revoking a refresh token
// cascades to its access token.
func (s *OAuth2ServerService) Revoke(ctx context.Context, client *model.OAuth2Client, rawToken, hint string) {
	hash := sha256Hex(rawToken)
	tctx := tenantCtx(ctx, client.TenantID)
	// Try refresh first (cascades), unless hint says access_token.
	if hint != "access_token" {
		if refresh, err := s.tokens.GetRefreshByHashContext(ctx, hash); err == nil {
			if refresh.ClientID == client.ClientID {
				_, _ = s.tokens.RevokeRefreshByHashContext(tctx, hash)
				_ = s.tokens.RevokeAccessByRefreshTokenIDContext(tctx, refresh.ID)
			}
			return
		}
	}
	if access, err := s.tokens.GetAccessByHashContext(ctx, hash); err == nil {
		if access.ClientID == client.ClientID {
			_, _ = s.tokens.RevokeAccessByHashContext(tctx, hash)
		}
	}
}

// UserInfo returns the profile claims allowed by the token's scopes.
func (s *OAuth2ServerService) UserInfo(ctx context.Context, rawToken string) (map[string]any, *OAuth2Error) {
	token, err := s.lookupActiveAccessToken(ctx, rawToken)
	if err != nil {
		return nil, oauth2Err(401, "invalid_token", "access token is invalid or expired")
	}
	if token.UserID == nil {
		return nil, oauth2Err(403, "insufficient_scope", "token has no associated user")
	}
	user, err := s.users.GetUserWithRolesContext(tenantCtx(ctx, token.TenantID), *token.UserID)
	if err != nil {
		return nil, oauth2Err(500, "server_error", "failed to load user")
	}
	claims := map[string]any{"sub": user.ID}
	if containsStr(token.Scopes, "profile") {
		claims["username"] = user.Username
		claims["nickname"] = user.Nickname
		claims["avatar"] = user.Avatar
	}
	if containsStr(token.Scopes, "email") {
		claims["email"] = user.Email
	}
	// When no recognized scopes, still expose the stable subject id.
	return claims, nil
}
