package wxclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

type failingRoundTripper func(*http.Request) (*http.Response, error)

func (f failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func credentialLeakingClient() *http.Client {
	return &http.Client{Transport: failingRoundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed for " + req.URL.String())
	})}
}

func assertNoCredentialLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected request error")
	}
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked credential %q: %v", secret, err)
		}
	}
}

func TestTokenRequestErrorDoesNotLeakSecret(t *testing.T) {
	mgr := NewTokenManager(
		newMemCache(),
		staticResolver{appID: "wx-sensitive-app", secret: "SECRET-SHOULD-NOT-LEAK"},
		WithTokenHTTPClient(credentialLeakingClient()),
		WithTokenBaseURL("https://wechat.invalid"),
	)
	_, err := mgr.GetToken(context.Background(), &store.MpAccount{AppID: "wx-sensitive-app"})
	assertNoCredentialLeak(t, err, "SECRET-SHOULD-NOT-LEAK", "secret=", "wechat.invalid")
}

func TestBusinessRequestErrorDoesNotLeakAccessToken(t *testing.T) {
	tokens := NewTokenManager(
		newMemCache(),
		staticResolver{appID: "wx-sensitive-app", secret: "safe"},
		WithTokenBaseURL("https://unused.invalid"),
	)
	if err := tokens.cache.Set(context.Background(), tokenKey("wx-sensitive-app"), "ACCESS-TOKEN-SHOULD-NOT-LEAK", 0); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	client := NewClient(tokens, WithBaseURL("https://wechat.invalid"), WithHTTPClient(credentialLeakingClient()))
	err := client.SendCustomerText(
		context.Background(),
		&store.MpAccount{AppID: "wx-sensitive-app"},
		"openid-safe",
		"hello",
	)
	assertNoCredentialLeak(t, err, "ACCESS-TOKEN-SHOULD-NOT-LEAK", "access_token=", "wechat.invalid")
}

func TestOAuthRequestErrorDoesNotLeakSecret(t *testing.T) {
	client := NewClient(nil, WithBaseURL("https://wechat.invalid"), WithHTTPClient(credentialLeakingClient()))
	_, err := client.ExchangeCodeForOpenID(
		context.Background(),
		"wx-sensitive-app",
		"OAUTH-SECRET-SHOULD-NOT-LEAK",
		"OAUTH-CODE-SHOULD-NOT-LEAK",
	)
	assertNoCredentialLeak(t, err, "OAUTH-SECRET-SHOULD-NOT-LEAK", "OAUTH-CODE-SHOULD-NOT-LEAK", "wechat.invalid")
}

func TestBusinessHTTPErrorCannotPassAsSuccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty body"},
		{name: "empty object", body: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			req, err := http.NewRequest(http.MethodPost, srv.URL+"?access_token=STATUS-TOKEN-SHOULD-NOT-LEAK", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			client := NewClient(nil, WithHTTPClient(srv.Client()))
			err = client.send(req, nil)
			if err == nil || !strings.Contains(err.Error(), "status 500") {
				t.Fatalf("HTTP 500 body %q returned %v", tc.body, err)
			}
			assertNoCredentialLeak(t, err, "STATUS-TOKEN-SHOULD-NOT-LEAK", "access_token=")
		})
	}
}

func TestCredentialEndpointsRejectSuccessfulPayloadOnHTTPError(t *testing.T) {
	t.Run("token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"access_token":"unexpected","expires_in":7200}`))
		}))
		defer srv.Close()
		mgr := NewTokenManager(
			newMemCache(), staticResolver{appID: "wx-status", secret: "safe"},
			WithTokenHTTPClient(srv.Client()), WithTokenBaseURL(srv.URL),
		)
		if _, err := mgr.GetToken(context.Background(), &store.MpAccount{AppID: "wx-status"}); err == nil {
			t.Fatal("token endpoint accepted HTTP 500")
		}
	})

	t.Run("oauth", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"access_token":"unexpected","openid":"unexpected"}`))
		}))
		defer srv.Close()
		client := NewClient(nil, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
		if _, err := client.ExchangeCodeForOpenID(context.Background(), "wx-status", "safe", "code"); err == nil {
			t.Fatal("oauth endpoint accepted HTTP 500")
		}
	})
}
