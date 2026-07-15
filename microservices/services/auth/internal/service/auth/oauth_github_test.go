package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
)

func TestGithubOAuthClientAuthURLStoresStateAndPKCE(t *testing.T) {
	store := &stubOAuthStateStore{}
	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore:   store,
		authorizeURL: "https://github.example.test/login/oauth/authorize",
	}

	authURL, err := client.AuthURLContext(context.Background())
	if err != nil {
		t.Fatalf("AuthURLContext() error = %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	query := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "github.example.test" || parsed.Path != "/login/oauth/authorize" {
		t.Fatalf("auth URL target = %s, want configured GitHub authorize URL", parsed.String())
	}
	if query.Get("client_id") != "github-client-id" {
		t.Fatalf("client_id = %q, want github-client-id", query.Get("client_id"))
	}
	if query.Get("redirect_uri") != "https://app.example.test/api/v1/oauth/github/callback" {
		t.Fatalf("redirect_uri = %q, want configured callback", query.Get("redirect_uri"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", query.Get("code_challenge_method"))
	}
	if len(query.Get("code_challenge")) != 43 {
		t.Fatalf("code_challenge length = %d, want 43", len(query.Get("code_challenge")))
	}
	if query.Get("state") == "" {
		t.Fatal("state query parameter must be set")
	}
	if store.state != query.Get("state") {
		t.Fatalf("stored state = %q, want URL state %q", store.state, query.Get("state"))
	}
	if len(store.verifier) < 43 || len(store.verifier) > 128 {
		t.Fatalf("stored code verifier length = %d, want RFC 7636 length range", len(store.verifier))
	}
	if store.ttl != oauthStateExpire {
		t.Fatalf("state ttl = %s, want %s", store.ttl, oauthStateExpire)
	}
}

func TestGithubOAuthClientAuthURLRetriesStateCollision(t *testing.T) {
	store := &stubOAuthStateStore{
		storeErrs: []error{cache.ErrOAuthStateAlreadyExists, nil},
	}
	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore:   store,
		authorizeURL: "https://github.example.test/login/oauth/authorize",
	}

	if _, err := client.AuthURLContext(context.Background()); err != nil {
		t.Fatalf("AuthURLContext() error = %v", err)
	}
	if store.storeCalls != 2 {
		t.Fatalf("state store calls = %d, want retry after collision", store.storeCalls)
	}
}

func TestGithubOAuthClientAuthURLMapsStateStoreFailureToProviderUnavailable(t *testing.T) {
	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore: &stubOAuthStateStore{
			storeErr: errors.New("redis is down"),
		},
		authorizeURL: "https://github.example.test/login/oauth/authorize",
	}

	_, err := client.AuthURLContext(context.Background())
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("AuthURLContext() error = %v, want ErrOAuthProviderUnavailable", err)
	}
}

func TestOAuthServiceGetGithubAuthURLContextUsesRealProviderWhenConfigReady(t *testing.T) {
	oldOAuth := config.Cfg.OAuth
	config.Cfg.OAuth.Github = config.OAuthProviderConfig{
		Enabled:      true,
		ClientID:     "github-client-id",
		ClientSecret: "github-client-secret",
		RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
	}
	t.Cleanup(func() {
		config.Cfg.OAuth = oldOAuth
	})

	store := &stubOAuthStateStore{}
	svc := &OAuthService{stateStore: store}

	authURL, err := svc.GetGithubAuthURLContext(context.Background())
	if err != nil {
		t.Fatalf("GetGithubAuthURLContext() error = %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Host != "github.com" || parsed.Path != "/login/oauth/authorize" {
		t.Fatalf("auth URL = %s, want GitHub authorize endpoint", parsed.String())
	}
	if parsed.Query().Get("state") == "" || store.state == "" {
		t.Fatalf("state was not generated and stored: url=%q store=%q", parsed.Query().Get("state"), store.state)
	}
}

func TestGithubOAuthClientResolveIdentityExchangesCodeWithVerifierAndFetchesUser(t *testing.T) {
	var tokenRequestSeen bool
	var userRequestSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			tokenRequestSeen = true
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s, want POST", r.Method)
			}
			if r.Header.Get("Accept") != "application/json" {
				t.Fatalf("token Accept = %q, want application/json", r.Header.Get("Accept"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			wantForm := map[string]string{
				"client_id":     "github-client-id",
				"client_secret": "github-client-secret",
				"code":          "oauth-code",
				"redirect_uri":  "https://app.example.test/api/v1/oauth/github/callback",
				"code_verifier": "stored-code-verifier",
			}
			for key, want := range wantForm {
				if got := r.Form.Get(key); got != want {
					t.Fatalf("token form %s = %q, want %q", key, got, want)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"github-access-token","token_type":"bearer","scope":"read:user"}`))
		case "/user":
			userRequestSeen = true
			if got := r.Header.Get("Authorization"); got != "Bearer github-access-token" {
				t.Fatalf("user Authorization = %q, want bearer token", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":123456,"login":"octocat","email":"octocat@example.com","avatar_url":"https://avatars.example.test/u/123456"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore: &stubOAuthStateStore{
			consumeState:    "returned-state",
			consumeVerifier: "stored-code-verifier",
		},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/login/oauth/access_token",
		userURL:    server.URL + "/user",
	}

	identity, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "returned-state")
	if err != nil {
		t.Fatalf("ResolveIdentityContext() error = %v", err)
	}
	if !tokenRequestSeen || !userRequestSeen {
		t.Fatalf("token request seen=%v user request seen=%v, want both", tokenRequestSeen, userRequestSeen)
	}
	if identity.Provider != "github" || identity.ProviderUserID != "123456" || identity.Username != "octocat" {
		t.Fatalf("identity = %#v, want GitHub octocat/123456", identity)
	}
	if identity.Email != "octocat@example.com" || identity.Avatar != "https://avatars.example.test/u/123456" {
		t.Fatalf("identity profile = %#v, want email/avatar from GitHub", identity)
	}
}

func TestGithubOAuthClientResolveIdentityRejectsInvalidStateBeforeTokenExchange(t *testing.T) {
	tokenCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore: &stubOAuthStateStore{
			consumeErr: errOAuthStateNotFound,
		},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/login/oauth/access_token",
		userURL:    server.URL + "/user",
	}

	_, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "missing-state")
	var validationErr OAuthValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "state" {
		t.Fatalf("ResolveIdentityContext() error = %T/%v, want state OAuthValidationError", err, err)
	}
	if tokenCalled {
		t.Fatal("ResolveIdentityContext must not exchange code when state is invalid")
	}
}

func TestGithubOAuthClientResolveIdentitySynthesizesEmailWhenGitHubEmailIsPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"github-access-token","token_type":"bearer"}`))
		case "/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":789,"login":"private-mail-user","email":null}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := githubOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/github/callback",
		},
		stateStore: &stubOAuthStateStore{consumeVerifier: "stored-code-verifier"},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/login/oauth/access_token",
		userURL:    server.URL + "/user",
	}

	identity, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "state")
	if err != nil {
		t.Fatalf("ResolveIdentityContext() error = %v", err)
	}
	if identity.Email != "github-789@oauth.local" {
		t.Fatalf("email = %q, want stable synthetic address", identity.Email)
	}
}

type stubOAuthStateStore struct {
	state           string
	verifier        string
	ttl             time.Duration
	storeErr        error
	storeErrs       []error
	storeCalls      int
	consumeState    string
	consumeVerifier string
	consumeErr      error
}

func (s *stubOAuthStateStore) StoreOAuthStateContext(ctx context.Context, state, verifier string, ttl time.Duration) error {
	s.storeCalls++
	s.state = state
	s.verifier = verifier
	s.ttl = ttl
	if len(s.storeErrs) >= s.storeCalls {
		return s.storeErrs[s.storeCalls-1]
	}
	return s.storeErr
}

func (s *stubOAuthStateStore) ConsumeOAuthStateContext(ctx context.Context, state string) (string, error) {
	s.consumeState = state
	if s.consumeErr != nil {
		return "", s.consumeErr
	}
	verifier := s.consumeVerifier
	if strings.TrimSpace(verifier) == "" {
		verifier = "stored-code-verifier"
	}
	return verifier, nil
}
