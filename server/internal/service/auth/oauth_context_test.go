package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/runtimeconfig"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func TestOAuthServiceFindOrCreateUserContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewOAuthServiceWithDB(db).findOrCreateUserContext(ctx, "github", "123", "alice", "alice@example.com", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("findOrCreateUserContext() error = %v, want context.Canceled", err)
	}
}

func TestOAuthServiceFindOrCreateUserContextUsesInjectedStoresForExistingBinding(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		binding: &model.OAuthBinding{UserID: 42},
	}
	users := &stubOAuthUserStore{
		user: &model.User{ID: 42, Username: "alice"},
	}
	svc := &OAuthService{
		bindingDAO: bindings,
		userDAO:    users,
	}

	got, err := svc.findOrCreateUserContext(context.Background(), "github", "123", "alice", "alice@example.com", "")
	if err != nil {
		t.Fatalf("findOrCreateUserContext() error = %v", err)
	}
	if got != users.user {
		t.Fatalf("findOrCreateUserContext() user = %#v, want injected user", got)
	}
	if bindings.getProvider != "github" || bindings.getProviderUserID != "123" {
		t.Fatalf("binding lookup = (%q, %q), want (github, 123)", bindings.getProvider, bindings.getProviderUserID)
	}
	if users.getID != 42 {
		t.Fatalf("user lookup id = %d, want 42", users.getID)
	}
	if bindings.createCalled || users.createCalled {
		t.Fatal("existing binding path must not create a user or binding")
	}
}

func TestOAuthServiceGithubCallbackContextRequiresTOTPForEnabledUser(t *testing.T) {
	setOAuthJWTTestConfig(t)

	svc := &OAuthService{
		bindingDAO: &stubOAuthBindingStore{binding: &model.OAuthBinding{UserID: 42}},
		userDAO: &stubOAuthUserStore{
			user: &model.User{ID: 42, Username: "alice", Status: 1, TOTPEnabled: true},
		},
		providerClients: map[string]oauthProviderClient{
			"github": fakeOAuthProviderClient(),
		},
	}

	resp, err := svc.GithubCallbackContext(context.Background(), "code", "state")
	if err != nil {
		t.Fatalf("GithubCallbackContext() error = %v", err)
	}
	if !resp.RequiresTOTP || resp.TOTPChallengeID == "" {
		t.Fatalf("GithubCallbackContext() requires_totp=%v challenge=%q, want challenge", resp.RequiresTOTP, resp.TOTPChallengeID)
	}
	if resp.AccessToken != "" || resp.RefreshToken != "" {
		t.Fatal("OAuth TOTP challenge response must not include session tokens")
	}
	claims, err := jwt.ParseTOTPChallenge(resp.TOTPChallengeID)
	if err != nil {
		t.Fatalf("ParseTOTPChallenge() error = %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" {
		t.Fatalf("challenge claims = %#v, want alice/42", claims)
	}
}

func TestOAuthServiceGithubCallbackContextRejectsDisabledUser(t *testing.T) {
	svc := &OAuthService{
		bindingDAO: &stubOAuthBindingStore{binding: &model.OAuthBinding{UserID: 42}},
		userDAO: &stubOAuthUserStore{
			user: &model.User{ID: 42, Username: "alice", Status: 2},
		},
		providerClients: map[string]oauthProviderClient{
			"github": fakeOAuthProviderClient(),
		},
	}

	_, err := svc.GithubCallbackContext(context.Background(), "code", "state")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("GithubCallbackContext() error = %v, want ErrUserDisabled", err)
	}
}

func TestOAuthServiceGithubCallbackContextMarksDefaultAdminPassword(t *testing.T) {
	setOAuthJWTTestConfig(t)
	oldSecurity := config.Cfg.Security
	config.Cfg.Security.DefaultAdmin.ForceChangePassword = true
	config.Cfg.Security.DefaultAdmin.DefaultUsername = "admin"
	t.Cleanup(func() {
		config.Cfg.Security = oldSecurity
	})

	users := &stubOAuthUserStore{
		user: &model.User{
			ID:       1,
			Username: "admin",
			Password: mustHashPasswordForTest(t, "admin123"),
			Status:   1,
		},
	}
	svc := &OAuthService{
		bindingDAO: &stubOAuthBindingStore{binding: &model.OAuthBinding{UserID: 1}},
		userDAO:    users,
		providerClients: map[string]oauthProviderClient{
			"github": fakeOAuthProviderClient(),
		},
	}

	resp, err := svc.GithubCallbackContext(context.Background(), "code", "state")
	if err != nil {
		t.Fatalf("GithubCallbackContext() error = %v", err)
	}
	if !users.markCalled {
		t.Fatal("default admin OAuth login should mark password change required")
	}
	if !resp.User.MustChangePassword {
		t.Fatal("OAuth response user should require password change")
	}
}

func TestOAuthServiceGithubCallbackContextUsesRuntimePasswordMaxAge(t *testing.T) {
	setOAuthJWTTestConfig(t)

	changedAt := time.Now().AddDate(0, 0, -60)
	users := &stubOAuthUserStore{
		user: &model.User{
			ID:                42,
			Username:          "alice",
			Password:          mustHashPasswordForTest(t, "CurrentPass1"),
			Status:            1,
			PasswordChangedAt: &changedAt,
		},
	}
	svc := &OAuthService{
		bindingDAO: &stubOAuthBindingStore{binding: &model.OAuthBinding{UserID: 42}},
		userDAO:    users,
		providerClients: map[string]oauthProviderClient{
			"github": fakeOAuthProviderClient(),
		},
		policyReader: stubSecurityPolicyReader{policy: runtimeconfig.SecurityPolicy{PasswordMaxAgeDays: 30}},
	}

	resp, err := svc.GithubCallbackContext(context.Background(), "code", "state")
	if err != nil {
		t.Fatalf("GithubCallbackContext() error = %v", err)
	}
	if !users.markCalled {
		t.Fatal("runtime password max age should mark OAuth user password change required")
	}
	if !resp.User.MustChangePassword {
		t.Fatal("OAuth response user should require password change")
	}
}

func TestOAuthServiceBindOAuthContextRejectsProviderUserBoundToAnotherUser(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		binding: &model.OAuthBinding{UserID: 99, Provider: "github", ProviderUserID: "123456"},
	}
	svc := &OAuthService{bindingDAO: bindings, providerClients: map[string]oauthProviderClient{"github": fakeOAuthProviderClient()}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider: "github",
		Code:     "oauth-code",
	})
	if !errors.Is(err, ErrOAuthBoundByAnotherUser) {
		t.Fatalf("BindOAuthContext() error = %v, want ErrOAuthBoundByAnotherUser", err)
	}
	if bindings.createCalled {
		t.Fatal("BindOAuthContext must not create when provider user belongs to another account")
	}
}

func TestOAuthServiceBindOAuthContextRejectsExistingProviderForUser(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		getErr:              gorm.ErrRecordNotFound,
		userProviderBinding: &model.OAuthBinding{UserID: 42, Provider: "github", ProviderUserID: "existing"},
	}
	svc := &OAuthService{bindingDAO: bindings, providerClients: map[string]oauthProviderClient{"github": fakeOAuthProviderClient()}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider: "github",
		Code:     "oauth-code",
	})
	if !errors.Is(err, ErrOAuthAlreadyBound) {
		t.Fatalf("BindOAuthContext() error = %v, want ErrOAuthAlreadyBound", err)
	}
	if bindings.createCalled {
		t.Fatal("BindOAuthContext must not create duplicate provider bindings for the same user")
	}
}

func TestOAuthServiceBindOAuthContextCreatesCurrentUserBinding(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		getErr:             gorm.ErrRecordNotFound,
		getUserProviderErr: gorm.ErrRecordNotFound,
	}
	svc := &OAuthService{bindingDAO: bindings, providerClients: map[string]oauthProviderClient{"github": fakeOAuthProviderClient()}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider: " GitHub ",
		Code:     "oauth-code",
	})
	if err != nil {
		t.Fatalf("BindOAuthContext() error = %v", err)
	}
	if bindings.binding == nil {
		t.Fatal("BindOAuthContext did not create binding")
	}
	if bindings.binding.UserID != 42 || bindings.binding.Provider != "github" || bindings.binding.ProviderUserID != "123456" {
		t.Fatalf("created binding = %#v, want current user/github/123456", bindings.binding)
	}
}

func TestOAuthServiceBindOAuthContextIgnoresClientSuppliedProviderUserID(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		getErr:             gorm.ErrRecordNotFound,
		getUserProviderErr: gorm.ErrRecordNotFound,
	}
	svc := &OAuthService{bindingDAO: bindings, providerClients: map[string]oauthProviderClient{"github": fakeOAuthProviderClient()}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider:       "github",
		Code:           "oauth-code",
		ProviderUserID: "attacker-controlled-id",
	})
	if err != nil {
		t.Fatalf("BindOAuthContext() error = %v", err)
	}
	if bindings.getProviderUserID == "attacker-controlled-id" {
		t.Fatal("BindOAuthContext must not trust client-supplied provider_user_id")
	}
	if bindings.binding.ProviderUserID != "123456" {
		t.Fatalf("provider user id = %q, want server-resolved 123456", bindings.binding.ProviderUserID)
	}
}

func TestOAuthServiceBindOAuthContextRequiresAuthorizationCode(t *testing.T) {
	svc := &OAuthService{bindingDAO: &stubOAuthBindingStore{}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{Provider: "github"})
	var validationErr OAuthValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "code" {
		t.Fatalf("BindOAuthContext() error = %T/%v, want code OAuthValidationError", err, err)
	}
}

func TestOAuthServiceGithubCallbackContextFailsClosedWithoutProviderClient(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)
	_, err := NewOAuthServiceWithDB(db).GithubCallbackContext(context.Background(), "oauth-code", "state")
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("GithubCallbackContext() error = %v, want ErrOAuthProviderUnavailable", err)
	}
}

func TestOAuthServiceGetGithubAuthURLFailsClosedWithoutProviderClient(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)
	_, err := NewOAuthServiceWithDB(db).GetGithubAuthURLContext(context.Background())
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("GetGithubAuthURL() error = %v, want ErrOAuthProviderUnavailable", err)
	}
}

func TestOAuthServiceBindOAuthContextFailsClosedWithoutProviderClient(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)
	err := NewOAuthServiceWithDB(db).BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider: "github",
		Code:     "oauth-code",
	})
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("BindOAuthContext() error = %v, want ErrOAuthProviderUnavailable", err)
	}
}

func TestOAuthServiceBindOAuthContextMapsDuplicateCreateToAlreadyBound(t *testing.T) {
	bindings := &stubOAuthBindingStore{
		getErr:             gorm.ErrRecordNotFound,
		getUserProviderErr: gorm.ErrRecordNotFound,
		createErr: &mysql.MySQLError{
			Number:  1062,
			Message: "Duplicate entry '42-github' for key 'oauth_bindings.uk_oauth_bindings_user_provider'",
		},
	}
	svc := &OAuthService{bindingDAO: bindings, providerClients: map[string]oauthProviderClient{"github": fakeOAuthProviderClient()}}

	err := svc.BindOAuthContext(context.Background(), 42, BindOAuthRequest{
		Provider: "github",
		Code:     "oauth-code",
	})
	if !errors.Is(err, ErrOAuthAlreadyBound) {
		t.Fatalf("BindOAuthContext() error = %v, want ErrOAuthAlreadyBound", err)
	}
}

func TestOAuthServiceUnbindOAuthContextReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	bindings := &stubOAuthBindingStore{deleteRows: 0}
	svc := &OAuthService{bindingDAO: bindings}

	err := svc.UnbindOAuthContext(context.Background(), 42, UnbindOAuthRequest{
		Provider: "github",
	})
	if !errors.Is(err, ErrOAuthBindingNotFound) {
		t.Fatalf("UnbindOAuthContext() error = %v, want ErrOAuthBindingNotFound", err)
	}
}

type stubOAuthBindingStore struct {
	binding                 *model.OAuthBinding
	userProviderBinding     *model.OAuthBinding
	getErr                  error
	getUserProviderErr      error
	createErr               error
	deleteErr               error
	deleteRows              int64
	getProvider             string
	getProviderUserID       string
	getUserProviderUserID   uint
	getUserProviderProvider string
	createCalled            bool
	deleteCalled            bool
}

func (s *stubOAuthBindingStore) GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error) {
	s.getProvider = provider
	s.getProviderUserID = providerUserID
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.binding == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.binding, nil
}

func (s *stubOAuthBindingStore) CreateContext(ctx context.Context, binding *model.OAuthBinding) error {
	s.createCalled = true
	if s.createErr != nil {
		return s.createErr
	}
	s.binding = binding
	return nil
}

func (s *stubOAuthBindingStore) GetByUserProviderContext(ctx context.Context, userID uint, provider string) (*model.OAuthBinding, error) {
	s.getUserProviderUserID = userID
	s.getUserProviderProvider = provider
	if s.getUserProviderErr != nil {
		return nil, s.getUserProviderErr
	}
	if s.userProviderBinding == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.userProviderBinding, nil
}

func (s *stubOAuthBindingStore) DeleteByUserProviderContext(ctx context.Context, userID uint, provider string) (int64, error) {
	s.deleteCalled = true
	s.getUserProviderUserID = userID
	s.getUserProviderProvider = provider
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	return s.deleteRows, nil
}

type stubOAuthUserStore struct {
	user         *model.User
	getErr       error
	markErr      error
	createErr    error
	getID        uint
	createCalled bool
	markCalled   bool
}

func (s *stubOAuthUserStore) GetUserByIDContext(ctx context.Context, id uint) (*model.User, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.user, nil
}

func (s *stubOAuthUserStore) CreateUserContext(ctx context.Context, user *model.User) error {
	s.createCalled = true
	if s.createErr != nil {
		return s.createErr
	}
	if user.ID == 0 {
		user.ID = 100
	}
	s.user = user
	return nil
}

func (s *stubOAuthUserStore) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.user, nil
}

func (s *stubOAuthUserStore) MarkPasswordChangeRequiredContext(ctx context.Context, userID uint) error {
	s.markCalled = true
	if s.markErr != nil {
		return s.markErr
	}
	if s.user != nil {
		s.user.MustChangePassword = true
	}
	return nil
}

type stubOAuthProviderClient struct {
	identity *oauthIdentity
}

func fakeOAuthProviderClient() stubOAuthProviderClient {
	return stubOAuthProviderClient{
		identity: &oauthIdentity{
			Provider:       "github",
			ProviderUserID: "123456",
			Username:       "testuser",
			Email:          "test@example.com",
			Avatar:         "https://avatars.githubusercontent.com/u/123456?v=4",
		},
	}
}

func (s stubOAuthProviderClient) AuthURLContext(ctx context.Context) (string, error) {
	return "https://provider.example.test/oauth", nil
}

func (s stubOAuthProviderClient) ResolveIdentityContext(ctx context.Context, code, state string) (*oauthIdentity, error) {
	if strings.TrimSpace(code) == "" {
		return nil, OAuthValidationError{Field: "code", Message: "authorization code is required"}
	}
	return s.identity, nil
}

func setOAuthJWTTestConfig(t *testing.T) {
	t.Helper()
	oldConfig := config.Cfg.JWT
	config.Cfg.JWT.Secret = "local-dev-secret-for-oauth-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	config.Cfg.JWT.AccessTokenExpire = int(time.Hour.Seconds())
	config.Cfg.JWT.RefreshTokenExpire = int((24 * time.Hour).Seconds())
	t.Cleanup(func() {
		config.Cfg.JWT = oldConfig
	})
}
