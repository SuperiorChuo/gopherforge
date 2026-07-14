package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-admin-kit/services/auth/internal/config"
)

func TestWechatOAuthClientAuthURLStoresState(t *testing.T) {
	store := &stubOAuthStateStore{}
	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore:   store,
		authorizeURL: "https://wechat.example.test/connect/qrconnect",
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
	if parsed.Scheme != "https" || parsed.Host != "wechat.example.test" || parsed.Path != "/connect/qrconnect" {
		t.Fatalf("auth URL target = %s, want configured WeChat authorize URL", parsed.String())
	}
	if query.Get("appid") != "wechat-app-id" {
		t.Fatalf("appid = %q, want wechat-app-id", query.Get("appid"))
	}
	if query.Get("redirect_uri") != "https://app.example.test/api/v1/oauth/wechat/callback" {
		t.Fatalf("redirect_uri = %q, want configured callback", query.Get("redirect_uri"))
	}
	if query.Get("response_type") != "code" {
		t.Fatalf("response_type = %q, want code", query.Get("response_type"))
	}
	if query.Get("scope") != "snsapi_login" {
		t.Fatalf("scope = %q, want snsapi_login", query.Get("scope"))
	}
	if query.Get("state") == "" || store.state != query.Get("state") {
		t.Fatalf("state was not generated and stored: url=%q store=%q", query.Get("state"), store.state)
	}
	if parsed.Fragment != "wechat_redirect" {
		t.Fatalf("fragment = %q, want wechat_redirect", parsed.Fragment)
	}
}

func TestOAuthServiceGetWechatAuthURLContextUsesRealProviderWhenConfigReady(t *testing.T) {
	oldOAuth := config.Cfg.OAuth
	config.Cfg.OAuth.Wechat = config.OAuthProviderConfig{
		Enabled:      true,
		ClientID:     "wechat-app-id",
		ClientSecret: "wechat-app-secret",
		RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
	}
	t.Cleanup(func() {
		config.Cfg.OAuth = oldOAuth
	})

	store := &stubOAuthStateStore{}
	svc := &OAuthService{stateStore: store}

	authURL, err := svc.GetWechatAuthURLContext(context.Background())
	if err != nil {
		t.Fatalf("GetWechatAuthURLContext() error = %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Host != "open.weixin.qq.com" || parsed.Path != "/connect/qrconnect" {
		t.Fatalf("auth URL = %s, want WeChat QR connect endpoint", parsed.String())
	}
	if parsed.Query().Get("state") == "" || store.state == "" {
		t.Fatalf("state was not generated and stored: url=%q store=%q", parsed.Query().Get("state"), store.state)
	}
}

func TestWechatOAuthClientResolveIdentityExchangesCodeAndFetchesUser(t *testing.T) {
	var tokenRequestSeen bool
	var userRequestSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sns/oauth2/access_token":
			tokenRequestSeen = true
			query := r.URL.Query()
			want := map[string]string{
				"appid":      "wechat-app-id",
				"secret":     "wechat-app-secret",
				"code":       "oauth-code",
				"grant_type": "authorization_code",
			}
			for key, expected := range want {
				if got := query.Get(key); got != expected {
					t.Fatalf("token query %s = %q, want %q", key, got, expected)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"wechat-access-token","openid":"openid-123","unionid":"union-456","scope":"snsapi_login"}`))
		case "/sns/userinfo":
			userRequestSeen = true
			query := r.URL.Query()
			if query.Get("access_token") != "wechat-access-token" || query.Get("openid") != "openid-123" {
				t.Fatalf("userinfo query = %s, want access token and openid", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"openid":"openid-123","unionid":"union-456","nickname":"WeChat User","headimgurl":"https://avatar.example.test/wechat.png"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore: &stubOAuthStateStore{consumeVerifier: "stored-state-nonce"},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/sns/oauth2/access_token",
		userURL:    server.URL + "/sns/userinfo",
	}

	identity, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "returned-state")
	if err != nil {
		t.Fatalf("ResolveIdentityContext() error = %v", err)
	}
	if !tokenRequestSeen || !userRequestSeen {
		t.Fatalf("token request seen=%v user request seen=%v, want both", tokenRequestSeen, userRequestSeen)
	}
	if identity.Provider != "wechat" || identity.ProviderUserID != "union-456" || identity.Username != "WeChat User" {
		t.Fatalf("identity = %#v, want WeChat user union-456", identity)
	}
	if identity.Email != "wechat-union-456@oauth.local" || identity.Avatar != "https://avatar.example.test/wechat.png" {
		t.Fatalf("identity profile = %#v, want synthetic email/avatar", identity)
	}
}

func TestWechatOAuthClientResolveIdentityRejectsMismatchedUserInfoOpenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sns/oauth2/access_token":
			_, _ = w.Write([]byte(`{"access_token":"wechat-access-token","openid":"token-openid","unionid":"union-456","scope":"snsapi_login"}`))
		case "/sns/userinfo":
			_, _ = w.Write([]byte(`{"openid":"userinfo-openid","unionid":"union-456","nickname":"WeChat User"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore: &stubOAuthStateStore{consumeVerifier: "stored-state-nonce"},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/sns/oauth2/access_token",
		userURL:    server.URL + "/sns/userinfo",
	}

	_, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "returned-state")
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("ResolveIdentityContext() error = %T/%v, want ErrOAuthProviderUnavailable", err, err)
	}
}

func TestWechatOAuthClientResolveIdentityRejectsMismatchedUserInfoUnionID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sns/oauth2/access_token":
			_, _ = w.Write([]byte(`{"access_token":"wechat-access-token","openid":"openid-123","unionid":"token-union","scope":"snsapi_login"}`))
		case "/sns/userinfo":
			_, _ = w.Write([]byte(`{"openid":"openid-123","unionid":"userinfo-union","nickname":"WeChat User"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore: &stubOAuthStateStore{consumeVerifier: "stored-state-nonce"},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/sns/oauth2/access_token",
		userURL:    server.URL + "/sns/userinfo",
	}

	_, err := client.ResolveIdentityContext(context.Background(), "oauth-code", "returned-state")
	if !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("ResolveIdentityContext() error = %T/%v, want ErrOAuthProviderUnavailable", err, err)
	}
}

func TestWechatOAuthClientResolveIdentityRejectsInvalidStateBeforeTokenExchange(t *testing.T) {
	tokenCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore: &stubOAuthStateStore{consumeErr: errOAuthStateNotFound},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL + "/sns/oauth2/access_token",
		userURL:    server.URL + "/sns/userinfo",
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

func TestWechatOAuthClientResolveIdentityMapsInvalidCodeToValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":40029,"errmsg":"invalid code"}`))
	}))
	defer server.Close()

	client := wechatOAuthClient{
		config: config.OAuthProviderConfig{
			Enabled:      true,
			ClientID:     "wechat-app-id",
			ClientSecret: "wechat-app-secret",
			RedirectURI:  "https://app.example.test/api/v1/oauth/wechat/callback",
		},
		stateStore: &stubOAuthStateStore{consumeVerifier: "stored-state-nonce"},
		httpClient: http.DefaultClient,
		tokenURL:   server.URL,
	}

	_, err := client.ResolveIdentityContext(context.Background(), "bad-code", "state")
	var validationErr OAuthValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "code" {
		t.Fatalf("ResolveIdentityContext() error = %T/%v, want code OAuthValidationError", err, err)
	}
}
