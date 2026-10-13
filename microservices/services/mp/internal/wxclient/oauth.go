package wxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// oauthAccessTokenPath 是网页授权换 token 端点。注意它在 /sns 下，且用 appsecret
// 直换，不带中控 access_token（与业务接口鉴权方式不同）。
const oauthAccessTokenPath = "/sns/oauth2/access_token"

// OAuthResult 是网页授权换取的结果：网页授权 access_token + openid（+ 可选 unionid）。
// 注意这里的 AccessToken 是「网页授权 access_token」，与中控的普通 access_token 不同，
// 只能用于拉取该用户的网页授权用户信息，不能调普通业务接口。
type OAuthResult struct {
	OpenID      string
	AccessToken string
	Scope       string
	UnionID     string
}

// oauthResp 是 /sns/oauth2/access_token 的响应；失败时带 errcode/errmsg。
type oauthResp struct {
	wxCommonResp
	AccessToken string `json:"access_token"`
	OpenID      string `json:"openid"`
	Scope       string `json:"scope"`
	UnionID     string `json:"unionid"`
}

// ExchangeCodeForOpenID 用网页授权回调拿到的 code 换 openid（及网页授权 access_token）。
// appID/secret 由调用方（api 层）解析后传入——因为该接口用 appsecret 直换、不走中控
// token，wxclient 拿不到 raw secret，故显式收参。
//
//	GET /sns/oauth2/access_token?appid=X&secret=Y&code=Z&grant_type=authorization_code
//	→ {"access_token":...,"openid":...,"scope":...,"unionid":...} 或 {"errcode":...}
func (c *Client) ExchangeCodeForOpenID(ctx context.Context, appID, secret, code string) (OAuthResult, error) {
	q := url.Values{}
	q.Set("appid", appID)
	q.Set("secret", secret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	reqURL := c.baseURL + oauthAccessTokenPath + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return OAuthResult{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return OAuthResult{}, safeRequestError("oauth exchange", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuthResult{}, fmt.Errorf("oauth exchange: read body (status %d): %w", resp.StatusCode, err)
	}

	var or oauthResp
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &or); err != nil {
			return OAuthResult{}, fmt.Errorf("oauth exchange: decode (status %d): %w", resp.StatusCode, err)
		}
	}
	if err := or.asWxError(); err != nil {
		return OAuthResult{}, err
	}
	if !successfulHTTPStatus(resp.StatusCode) {
		return OAuthResult{}, unexpectedHTTPStatus("oauth exchange", resp.StatusCode)
	}
	if or.OpenID == "" {
		return OAuthResult{}, fmt.Errorf("oauth exchange: empty openid in response")
	}
	return OAuthResult{
		OpenID:      or.OpenID,
		AccessToken: or.AccessToken,
		Scope:       or.Scope,
		UnionID:     or.UnionID,
	}, nil
}
