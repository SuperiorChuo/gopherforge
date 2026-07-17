package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func testCtx(t *testing.T, host string, mutate func(*http.Request)) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/v1/im/widget/config", nil)
	req.Host = host
	if mutate != nil {
		mutate(req)
	}
	c.Request = req
	return c
}

func TestSameHostOrigin(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{"same host http", "im.test", "http://im.test", true},
		{"same host https", "im.test", "https://im.test", true},
		{"case insensitive", "im.test", "http://IM.TEST", true},
		{"other host", "im.test", "http://other.test", false},
		{"port mismatch", "im.test", "http://im.test:8080", false},
		{"port match", "im.test:8088", "http://im.test:8088", true},
		{"garbage origin", "im.test", "not-a-url", false},
		{"empty origin", "im.test", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCtx(t, tc.host, nil)
			if got := sameHostOrigin(c, tc.origin); got != tc.want {
				t.Fatalf("sameHostOrigin(host=%q, origin=%q) = %v, want %v", tc.host, tc.origin, got, tc.want)
			}
		})
	}
}

func TestOriginDenied(t *testing.T) {
	allowed := `["http://ok.test"]`
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"empty origin allowed", "", false},
		{"null origin allowed", "null", false},
		{"whitelisted", "http://ok.test", false},
		{"same host bypasses whitelist", "http://im.test", false},
		{"foreign origin denied", "http://evil.test", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCtx(t, "im.test", nil)
			if got := originDenied(c, allowed, tc.origin); got != tc.want {
				t.Fatalf("originDenied(origin=%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestParentOriginPrecedence(t *testing.T) {
	// body > query > X-Parent-Origin header > Origin header
	c := testCtx(t, "im.test", func(req *http.Request) {
		req.URL.RawQuery = "parent_origin=http%3A%2F%2Ffrom-query.test"
		req.Header.Set("X-Parent-Origin", "http://from-xheader.test")
		req.Header.Set("Origin", "http://from-origin.test")
	})
	if got := parentOrigin(c, "http://from-body.test"); got != "http://from-body.test" {
		t.Fatalf("body should win, got %q", got)
	}
	if got := parentOrigin(c, ""); got != "http://from-query.test" {
		t.Fatalf("query should win over headers, got %q", got)
	}

	c = testCtx(t, "im.test", func(req *http.Request) {
		req.Header.Set("X-Parent-Origin", "http://from-xheader.test")
		req.Header.Set("Origin", "http://from-origin.test")
	})
	if got := parentOrigin(c, ""); got != "http://from-xheader.test" {
		t.Fatalf("X-Parent-Origin should win over Origin, got %q", got)
	}

	c = testCtx(t, "im.test", func(req *http.Request) {
		req.Header.Set("Origin", "http://from-origin.test")
	})
	if got := parentOrigin(c, ""); got != "http://from-origin.test" {
		t.Fatalf("Origin fallback, got %q", got)
	}
}

func TestEmbedSnippet(t *testing.T) {
	c := testCtx(t, "im.test", nil)
	want := `<script src="http://im.test/im/widget/widget.js" data-app-key="demo" async></script>`
	if got := embedSnippet(c, "demo"); got != want {
		t.Fatalf("snippet = %q, want %q", got, want)
	}

	c = testCtx(t, "im.test", func(req *http.Request) {
		req.Header.Set("X-Forwarded-Proto", "https")
	})
	want = `<script src="https://im.test/im/widget/widget.js" data-app-key="demo" async></script>`
	if got := embedSnippet(c, "demo"); got != want {
		t.Fatalf("https snippet = %q, want %q", got, want)
	}
}
