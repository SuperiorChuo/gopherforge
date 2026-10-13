package wxclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExchangeCodeForOpenID 验证网页授权换 openid：走 /sns/oauth2/access_token、
// query 拼接（appid/secret/code/grant_type）正确、响应解析正确，且不带 access_token
// query（该接口用 appsecret 直换，不走中控 token）。
func TestExchangeCodeForOpenID(t *testing.T) {
	var gotPath string
	var gotAppID, gotSecret, gotCode, gotGrant, gotAccessTok string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		q := r.URL.Query()
		gotAppID = q.Get("appid")
		gotSecret = q.Get("secret")
		gotCode = q.Get("code")
		gotGrant = q.Get("grant_type")
		gotAccessTok = q.Get("access_token")
		_, _ = w.Write([]byte(`{"access_token":"WEB-TOK","openid":"OPENID-1","scope":"snsapi_userinfo","unionid":"UNION-1"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	res, err := c.ExchangeCodeForOpenID(context.Background(), "wxappid", "wxsecret", "CODE-abc")
	if err != nil {
		t.Fatalf("ExchangeCodeForOpenID: %v", err)
	}
	if gotPath != "/sns/oauth2/access_token" {
		t.Fatalf("path = %q, want /sns/oauth2/access_token", gotPath)
	}
	if gotAppID != "wxappid" || gotSecret != "wxsecret" || gotCode != "CODE-abc" {
		t.Fatalf("query appid/secret/code = %q/%q/%q", gotAppID, gotSecret, gotCode)
	}
	if gotGrant != "authorization_code" {
		t.Fatalf("grant_type = %q, want authorization_code", gotGrant)
	}
	if gotAccessTok != "" {
		t.Fatalf("oauth exchange should NOT carry access_token query, got %q", gotAccessTok)
	}
	if res.OpenID != "OPENID-1" {
		t.Fatalf("OpenID = %q, want OPENID-1", res.OpenID)
	}
	if res.AccessToken != "WEB-TOK" {
		t.Fatalf("AccessToken = %q, want WEB-TOK", res.AccessToken)
	}
	if res.Scope != "snsapi_userinfo" {
		t.Fatalf("Scope = %q", res.Scope)
	}
	if res.UnionID != "UNION-1" {
		t.Fatalf("UnionID = %q", res.UnionID)
	}
}

// TestExchangeCodeForOpenIDWxError 验证 code 无效（errcode 40029）转 WxError。
func TestExchangeCodeForOpenIDWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":40029,"errmsg":"invalid code"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ExchangeCodeForOpenID(context.Background(), "a", "s", "bad-code")
	we, ok := err.(*WxError)
	if !ok || we.Code != 40029 {
		t.Fatalf("expected WxError 40029, got %T %v", err, err)
	}
}

// TestExchangeCodeForOpenIDEmptyOpenID 验证响应无 errcode 但 openid 为空时报错（防御）。
func TestExchangeCodeForOpenIDEmptyOpenID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"WEB-TOK","scope":"snsapi_base"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ExchangeCodeForOpenID(context.Background(), "a", "s", "c")
	if err == nil {
		t.Fatal("expected error for empty openid, got nil")
	}
	if _, ok := err.(*WxError); ok {
		t.Fatalf("empty openid should not be a WxError, got %v", err)
	}
}
