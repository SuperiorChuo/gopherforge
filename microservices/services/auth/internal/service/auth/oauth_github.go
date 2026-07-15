package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/cache"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	oauthStateExpire   = cache.OAuthStateExpire
	oauthStateAttempts = 3
)

var (
	errOAuthStateNotFound      = cache.ErrOAuthStateNotFound
	errOAuthStateAlreadyExists = cache.ErrOAuthStateAlreadyExists
)

type githubOAuthClient struct {
	config       config.OAuthProviderConfig
	stateStore   oauthStateStore
	httpClient   *http.Client
	authorizeURL string
	tokenURL     string
	userURL      string
}

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type githubUserResponse struct {
	ID        int64   `json:"id"`
	Login     string  `json:"login"`
	Email     *string `json:"email"`
	AvatarURL string  `json:"avatar_url"`
}

func newGithubOAuthClient(cfg config.OAuthProviderConfig, stateStore oauthStateStore) oauthProviderClient {
	return githubOAuthClient{
		config:       cfg,
		stateStore:   stateStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		authorizeURL: githubAuthorizeURL,
		tokenURL:     githubTokenURL,
		userURL:      githubUserURL,
	}
}

func (c githubOAuthClient) AuthURLContext(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !c.config.Ready() || c.stateStore == nil {
		return "", ErrOAuthProviderUnavailable
	}

	authURL := c.authorizeURL
	if authURL == "" {
		authURL = githubAuthorizeURL
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		return "", ErrOAuthProviderUnavailable
	}
	for range oauthStateAttempts {
		state, err := randomBase64URL(32)
		if err != nil {
			return "", err
		}
		verifier, err := randomBase64URL(32)
		if err != nil {
			return "", err
		}
		if err := c.stateStore.StoreOAuthStateContext(ctx, state, verifier, oauthStateExpire); err != nil {
			if errors.Is(err, errOAuthStateAlreadyExists) {
				continue
			}
			return "", ErrOAuthProviderUnavailable
		}

		query := parsed.Query()
		query.Set("client_id", c.config.ClientID)
		query.Set("redirect_uri", c.config.RedirectURI)
		query.Set("scope", "read:user")
		query.Set("state", state)
		query.Set("code_challenge", codeChallengeS256(verifier))
		query.Set("code_challenge_method", "S256")
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	return "", ErrOAuthProviderUnavailable
}

func (c githubOAuthClient) ResolveIdentityContext(ctx context.Context, code, state string) (*oauthIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	if code == "" {
		return nil, OAuthValidationError{Field: "code", Message: "authorization code is required"}
	}
	if state == "" {
		return nil, OAuthValidationError{Field: "state", Message: "oauth state is required"}
	}
	if !c.config.Ready() || c.stateStore == nil {
		return nil, ErrOAuthProviderUnavailable
	}

	verifier, err := c.stateStore.ConsumeOAuthStateContext(ctx, state)
	if errors.Is(err, errOAuthStateNotFound) {
		return nil, OAuthValidationError{Field: "state", Message: "invalid or expired oauth state"}
	}
	if err != nil {
		return nil, err
	}
	verifier = strings.TrimSpace(verifier)
	if verifier == "" {
		return nil, OAuthValidationError{Field: "state", Message: "invalid or expired oauth state"}
	}

	token, err := c.exchangeCodeContext(ctx, code, verifier)
	if err != nil {
		return nil, err
	}
	return c.fetchUserIdentityContext(ctx, token)
}

func (c githubOAuthClient) exchangeCodeContext(ctx context.Context, code, verifier string) (string, error) {
	tokenURL := c.tokenURL
	if tokenURL == "" {
		tokenURL = githubTokenURL
	}
	form := url.Values{}
	form.Set("client_id", c.config.ClientID)
	form.Set("client_secret", c.config.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.config.RedirectURI)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrOAuthProviderUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return "", oauthExternalRequestError(ctx, err)
	}
	defer resp.Body.Close()

	var tokenResp githubTokenResponse
	if err := decodeLimitedJSON(resp.Body, &tokenResp); err != nil {
		return "", ErrOAuthProviderUnavailable
	}
	if tokenResp.Error != "" {
		if tokenResp.Error == "bad_verification_code" {
			return "", OAuthValidationError{Field: "code", Message: "authorization code exchange failed"}
		}
		return "", ErrOAuthProviderUnavailable
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", ErrOAuthProviderUnavailable
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", ErrOAuthProviderUnavailable
	}
	return tokenResp.AccessToken, nil
}

func (c githubOAuthClient) fetchUserIdentityContext(ctx context.Context, accessToken string) (*oauthIdentity, error) {
	userURL := c.userURL
	if userURL == "" {
		userURL = githubUserURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return nil, oauthExternalRequestError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrOAuthProviderUnavailable
	}

	var userResp githubUserResponse
	if err := decodeLimitedJSON(resp.Body, &userResp); err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	if userResp.ID <= 0 {
		return nil, ErrOAuthProviderUnavailable
	}
	username := strings.TrimSpace(userResp.Login)
	if username == "" {
		username = fmt.Sprintf("github_%d", userResp.ID)
	}
	email := ""
	if userResp.Email != nil {
		email = strings.TrimSpace(*userResp.Email)
	}
	if email == "" {
		email = fmt.Sprintf("github-%d@oauth.local", userResp.ID)
	}

	return &oauthIdentity{
		Provider:       "github",
		ProviderUserID: strconv.FormatInt(userResp.ID, 10),
		Username:       username,
		Email:          email,
		Avatar:         userResp.AvatarURL,
	}, nil
}

func (c githubOAuthClient) httpClientOrDefault() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func randomBase64URL(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func oauthExternalRequestError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return ErrOAuthProviderUnavailable
}

func decodeLimitedJSON(reader io.Reader, target any) error {
	return json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(target)
}
