package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-admin-kit/services/auth/internal/config"
	authDAO "github.com/go-admin-kit/services/auth/internal/dao/auth"
	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type oauthBindingStore interface {
	GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error)
	GetByUserProviderContext(ctx context.Context, userID uint, provider string) (*model.OAuthBinding, error)
	CreateContext(ctx context.Context, binding *model.OAuthBinding) error
	DeleteByUserProviderContext(ctx context.Context, userID uint, provider string) (int64, error)
}

type oauthUserStore interface {
	GetUserByIDContext(ctx context.Context, id uint) (*model.User, error)
	GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error)
	MarkPasswordChangeRequiredContext(ctx context.Context, userID uint) error
	CreateUserContext(ctx context.Context, user *model.User) error
}

type OAuthService struct {
	bindingDAO      oauthBindingStore
	userDAO         oauthUserStore
	providerClients map[string]oauthProviderClient
	stateStore      oauthStateStore
	policyReader    runtimeconfig.SecurityPolicyReader
}

// NewOAuthServiceWithDB builds an OAuthService whose binding and user stores
// use an injected database handle. State store and provider clients keep
// their default implementations.
func NewOAuthServiceWithDB(db *gorm.DB) *OAuthService {
	return &OAuthService{
		bindingDAO: authDAO.NewOAuthBindingDAO(db),
		userDAO:    authDAO.NewUserDAO(db),
	}
}

type oauthProviderClient interface {
	AuthURLContext(ctx context.Context) (string, error)
	ResolveIdentityContext(ctx context.Context, code, state string) (*oauthIdentity, error)
}

type oauthStateStore interface {
	StoreOAuthStateContext(ctx context.Context, state, verifier string, expire time.Duration) error
	ConsumeOAuthStateContext(ctx context.Context, state string) (string, error)
}

type OAuthResponse struct {
	User            model.User `json:"user"`
	AccessToken     string     `json:"access_token"`
	RefreshToken    string     `json:"refresh_token"`
	RequiresTOTP    bool       `json:"requires_totp"`
	TOTPChallengeID string     `json:"totp_challenge_id,omitempty"`
}

var (
	ErrOAuthAlreadyBound        = errors.New("oauth provider already bound")
	ErrOAuthBoundByAnotherUser  = errors.New("oauth account already bound by another user")
	ErrOAuthBindingNotFound     = errors.New("oauth binding not found")
	ErrOAuthProviderUnavailable = errors.New("oauth provider unavailable")
)

type OAuthValidationError struct {
	Field   string
	Message string
}

func (e OAuthValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

type BindOAuthRequest struct {
	Provider       string `json:"provider"`
	Code           string `json:"code"`
	State          string `json:"state,omitempty"`
	ProviderUserID string `json:"provider_user_id,omitempty"`
	AccessToken    string `json:"access_token,omitempty"`
	RefreshToken   string `json:"refresh_token,omitempty"`
}

type oauthIdentity struct {
	Provider       string
	ProviderUserID string
	Username       string
	Email          string
	Avatar         string
}

type UnbindOAuthRequest struct {
	Provider string `json:"provider"`
}

func (s *OAuthService) GetGithubAuthURLContext(ctx context.Context) (string, error) {
	client, err := s.providerClient("github")
	if err != nil {
		return "", err
	}
	return client.AuthURLContext(ctx)
}

func (s *OAuthService) GithubCallbackContext(ctx context.Context, code, state string) (*OAuthResponse, error) {
	identity, err := s.resolveOAuthIdentityContext(ctx, "github", code, state)
	if err != nil {
		return nil, err
	}
	user, err := s.findOrCreateUserContext(ctx, identity.Provider, identity.ProviderUserID, identity.Username, identity.Email, identity.Avatar)
	if err != nil {
		return nil, err
	}

	return s.buildOAuthLoginResponseContext(ctx, user)
}

func (s *OAuthService) GetWechatAuthURLContext(ctx context.Context) (string, error) {
	client, err := s.providerClient("wechat")
	if err != nil {
		return "", err
	}
	return client.AuthURLContext(ctx)
}

func (s *OAuthService) WechatCallbackContext(ctx context.Context, code, state string) (*OAuthResponse, error) {
	identity, err := s.resolveOAuthIdentityContext(ctx, "wechat", code, state)
	if err != nil {
		return nil, err
	}
	user, err := s.findOrCreateUserContext(ctx, identity.Provider, identity.ProviderUserID, identity.Username, identity.Email, identity.Avatar)
	if err != nil {
		return nil, err
	}

	return s.buildOAuthLoginResponseContext(ctx, user)
}

func (s *OAuthService) BindOAuthContext(ctx context.Context, userID uint, req BindOAuthRequest) error {
	provider, err := normalizeOAuthProvider(req.Provider)
	if err != nil {
		return err
	}
	if userID == 0 {
		return ErrUserNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	identity, err := s.resolveOAuthIdentityContext(ctx, provider, req.Code, req.State)
	if err != nil {
		return err
	}

	bindingDAO := s.bindingStore()
	existingProviderUser, err := bindingDAO.GetByProviderUserContext(ctx, provider, identity.ProviderUserID)
	if err == nil {
		if existingProviderUser.UserID != userID {
			return ErrOAuthBoundByAnotherUser
		}
		return ErrOAuthAlreadyBound
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	existingUserProvider, err := bindingDAO.GetByUserProviderContext(ctx, userID, provider)
	if err == nil && existingUserProvider != nil {
		return ErrOAuthAlreadyBound
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err := bindingDAO.CreateContext(ctx, &model.OAuthBinding{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: identity.ProviderUserID,
	}); err != nil {
		return mapOAuthBindingCreateError(err)
	}
	return nil
}

func (s *OAuthService) UnbindOAuthContext(ctx context.Context, userID uint, req UnbindOAuthRequest) error {
	provider, err := normalizeOAuthProvider(req.Provider)
	if err != nil {
		return err
	}
	if userID == 0 {
		return ErrUserNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := s.bindingStore().DeleteByUserProviderContext(ctx, userID, provider)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrOAuthBindingNotFound
	}
	return nil
}

func (s *OAuthService) buildOAuthLoginResponseContext(ctx context.Context, user *model.User) (*OAuthResponse, error) {
	if user == nil {
		return nil, ErrUserNotFound
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	userDAO := s.userStore()
	policy := s.securityPolicy(ctx)
	passwordChangeRequired := hasDefaultAdminPassword(user) ||
		isPasswordExpired(user, time.Now(), policy.PasswordMaxAgeDays)
	if passwordChangeRequired && !user.MustChangePassword {
		if err := userDAO.MarkPasswordChangeRequiredContext(ctx, user.ID); err != nil {
			return nil, err
		}
		user.MustChangePassword = true
	}

	if userWithRoles, err := userDAO.GetUserWithRolesAndPermissionsContext(ctx, user.ID); err == nil {
		user = userWithRoles
	}
	if passwordChangeRequired {
		user.MustChangePassword = true
	}
	if user.TOTPEnabled {
		challengeID, err := jwt.GenerateTOTPChallenge(user.ID, user.Username, 5*time.Minute)
		if err != nil {
			return nil, err
		}
		return &OAuthResponse{
			User:            *user,
			RequiresTOTP:    true,
			TOTPChallengeID: challengeID,
		}, nil
	}

	tenantID := user.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	accessToken, refreshToken, err := jwt.GenerateTokenWithTenantPlatformAndAccessTTL(
		user.ID, user.Username, tenantID, user.IsPlatformAdmin, 0,
	)
	if err != nil {
		return nil, err
	}
	return &OAuthResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *OAuthService) findOrCreateUserContext(ctx context.Context, provider, providerUserID, username, email, avatar string) (*model.User, error) {
	bindingDAO := s.bindingStore()
	binding, err := bindingDAO.GetByProviderUserContext(ctx, provider, providerUserID)
	if err == nil {
		userDAO := s.userStore()
		user, err := userDAO.GetUserByIDContext(ctx, binding.UserID)
		if err != nil {
			return nil, err
		}
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	user := model.User{
		Username:          username,
		Password:          generateRandomPassword(),
		Email:             email,
		Avatar:            avatar,
		Status:            1,
		PasswordChangedAt: &now,
	}

	userDAO := s.userStore()
	if err := userDAO.CreateUserContext(ctx, &user); err != nil {
		return nil, err
	}

	binding = &model.OAuthBinding{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
	}
	if err := bindingDAO.CreateContext(ctx, binding); err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *OAuthService) bindingStore() oauthBindingStore {
	if s != nil && s.bindingDAO != nil {
		return s.bindingDAO
	}
	return authDAO.OAuthBindingDAO{}
}

func (s *OAuthService) resolveOAuthIdentityContext(ctx context.Context, provider, code, state string) (*oauthIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = strings.TrimSpace(state)
	if strings.TrimSpace(code) == "" {
		return nil, OAuthValidationError{Field: "code", Message: "authorization code is required"}
	}
	client, err := s.providerClient(provider)
	if err != nil {
		return nil, err
	}
	return client.ResolveIdentityContext(ctx, code, state)
}

func normalizeOAuthProvider(provider string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "github", "wechat":
		return normalized, nil
	case "":
		return "", OAuthValidationError{Field: "provider", Message: "provider is required"}
	default:
		return "", OAuthValidationError{Field: "provider", Message: "unsupported provider"}
	}
}

func mapOAuthBindingCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		constraint := strings.ToLower(pgErr.ConstraintName)
		if strings.Contains(constraint, "uk_oauth_bindings_user_provider") ||
			strings.Contains(constraint, "uk_oauth_bindings_provider_user") {
			return ErrOAuthAlreadyBound
		}
	}
	return err
}

func (s *OAuthService) providerClient(provider string) (oauthProviderClient, error) {
	if s != nil && s.providerClients != nil {
		if client := s.providerClients[provider]; client != nil {
			return client, nil
		}
	}
	cfg := oauthProviderConfig(provider)
	if !cfg.Ready() {
		return nil, ErrOAuthProviderUnavailable
	}
	if provider == "github" {
		return newGithubOAuthClient(cfg, s.stateStoreOrDefault()), nil
	}
	if provider == "wechat" {
		return newWechatOAuthClient(cfg, s.stateStoreOrDefault()), nil
	}
	return nil, ErrOAuthProviderUnavailable
}

func (s *OAuthService) stateStoreOrDefault() oauthStateStore {
	if s != nil && s.stateStore != nil {
		return s.stateStore
	}
	return cache.NewCacheService()
}

func oauthProviderConfig(provider string) config.OAuthProviderConfig {
	switch provider {
	case "github":
		return config.Cfg.OAuth.Github
	case "wechat":
		return config.Cfg.OAuth.Wechat
	default:
		return config.OAuthProviderConfig{}
	}
}

func (s *OAuthService) userStore() oauthUserStore {
	if s != nil && s.userDAO != nil {
		return s.userDAO
	}
	return authDAO.NewUserDAO(nil)
}

func (s *OAuthService) securityPolicy(ctx context.Context) runtimeconfig.SecurityPolicy {
	if s != nil && s.policyReader != nil {
		return s.policyReader.SecurityPolicy(ctx)
	}
	return runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(ctx)
}

func generateRandomPassword() string {
	password := "random_password_" + uuid.New().String()[:8]
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashedPassword)
}
