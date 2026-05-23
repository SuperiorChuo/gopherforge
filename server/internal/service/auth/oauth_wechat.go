package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/config"
)

const (
	wechatAuthorizeURL = "https://open.weixin.qq.com/connect/qrconnect"
	wechatTokenURL     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatUserURL      = "https://api.weixin.qq.com/sns/userinfo"
)

type wechatOAuthClient struct {
	config       config.OAuthProviderConfig
	stateStore   oauthStateStore
	httpClient   *http.Client
	authorizeURL string
	tokenURL     string
	userURL      string
}

type wechatTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

type wechatUserResponse struct {
	OpenID     string `json:"openid"`
	Nickname   string `json:"nickname"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func newWechatOAuthClient(cfg config.OAuthProviderConfig, stateStore oauthStateStore) oauthProviderClient {
	return wechatOAuthClient{
		config:       cfg,
		stateStore:   stateStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		authorizeURL: wechatAuthorizeURL,
		tokenURL:     wechatTokenURL,
		userURL:      wechatUserURL,
	}
}

func (c wechatOAuthClient) AuthURLContext(ctx context.Context) (string, error) {
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
		authURL = wechatAuthorizeURL
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
		nonce, err := randomBase64URL(32)
		if err != nil {
			return "", err
		}
		if err := c.stateStore.StoreOAuthStateContext(ctx, state, nonce, oauthStateExpire); err != nil {
			if errors.Is(err, errOAuthStateAlreadyExists) {
				continue
			}
			return "", ErrOAuthProviderUnavailable
		}

		query := parsed.Query()
		query.Set("appid", c.config.ClientID)
		query.Set("redirect_uri", c.config.RedirectURI)
		query.Set("response_type", "code")
		query.Set("scope", "snsapi_login")
		query.Set("state", state)
		parsed.RawQuery = query.Encode()
		parsed.Fragment = "wechat_redirect"
		return parsed.String(), nil
	}
	return "", ErrOAuthProviderUnavailable
}

func (c wechatOAuthClient) ResolveIdentityContext(ctx context.Context, code, state string) (*oauthIdentity, error) {
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

	nonce, err := c.stateStore.ConsumeOAuthStateContext(ctx, state)
	if errors.Is(err, errOAuthStateNotFound) {
		return nil, OAuthValidationError{Field: "state", Message: "invalid or expired oauth state"}
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(nonce) == "" {
		return nil, OAuthValidationError{Field: "state", Message: "invalid or expired oauth state"}
	}

	token, err := c.exchangeCodeContext(ctx, code)
	if err != nil {
		return nil, err
	}
	return c.fetchUserIdentityContext(ctx, token)
}

func (c wechatOAuthClient) exchangeCodeContext(ctx context.Context, code string) (*wechatTokenResponse, error) {
	tokenURL := c.tokenURL
	if tokenURL == "" {
		tokenURL = wechatTokenURL
	}
	parsed, err := url.Parse(tokenURL)
	if err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	query := parsed.Query()
	query.Set("appid", c.config.ClientID)
	query.Set("secret", c.config.ClientSecret)
	query.Set("code", code)
	query.Set("grant_type", "authorization_code")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return nil, oauthExternalRequestError(ctx, err)
	}
	defer resp.Body.Close()

	var tokenResp wechatTokenResponse
	if err := decodeLimitedJSON(resp.Body, &tokenResp); err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	if tokenResp.ErrCode != 0 {
		return nil, mapWechatError(tokenResp.ErrCode, "code")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrOAuthProviderUnavailable
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" || strings.TrimSpace(tokenResp.OpenID) == "" {
		return nil, ErrOAuthProviderUnavailable
	}
	return &tokenResp, nil
}

func (c wechatOAuthClient) fetchUserIdentityContext(ctx context.Context, token *wechatTokenResponse) (*oauthIdentity, error) {
	if token == nil {
		return nil, ErrOAuthProviderUnavailable
	}
	userURL := c.userURL
	if userURL == "" {
		userURL = wechatUserURL
	}
	parsed, err := url.Parse(userURL)
	if err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	query := parsed.Query()
	query.Set("access_token", token.AccessToken)
	query.Set("openid", token.OpenID)
	query.Set("lang", "zh_CN")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return nil, oauthExternalRequestError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrOAuthProviderUnavailable
	}

	var userResp wechatUserResponse
	if err := decodeLimitedJSON(resp.Body, &userResp); err != nil {
		return nil, ErrOAuthProviderUnavailable
	}
	if userResp.ErrCode != 0 {
		return nil, mapWechatError(userResp.ErrCode, "code")
	}

	openID := strings.TrimSpace(userResp.OpenID)
	tokenOpenID := strings.TrimSpace(token.OpenID)
	if openID == "" || openID != tokenOpenID {
		return nil, ErrOAuthProviderUnavailable
	}
	userUnionID := strings.TrimSpace(userResp.UnionID)
	tokenUnionID := strings.TrimSpace(token.UnionID)
	if userUnionID != "" && tokenUnionID != "" && userUnionID != tokenUnionID {
		return nil, ErrOAuthProviderUnavailable
	}
	providerUserID := firstNonEmpty(userUnionID, tokenUnionID, openID)
	if strings.TrimSpace(openID) == "" || strings.TrimSpace(providerUserID) == "" {
		return nil, ErrOAuthProviderUnavailable
	}
	username := strings.TrimSpace(userResp.Nickname)
	if username == "" {
		username = "wechat_" + safeOAuthLocalPart(providerUserID)
	}

	return &oauthIdentity{
		Provider:       "wechat",
		ProviderUserID: providerUserID,
		Username:       username,
		Email:          fmt.Sprintf("wechat-%s@oauth.local", safeOAuthLocalPart(providerUserID)),
		Avatar:         userResp.HeadImgURL,
	}, nil
}

func (c wechatOAuthClient) httpClientOrDefault() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func mapWechatError(code int, field string) error {
	switch code {
	case 40029, 40163:
		return OAuthValidationError{Field: field, Message: "authorization code exchange failed"}
	default:
		return ErrOAuthProviderUnavailable
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safeOAuthLocalPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
		if builder.Len() >= 48 {
			break
		}
	}
	safe := strings.Trim(builder.String(), "-_")
	if safe == "" {
		return "user"
	}
	return safe
}
