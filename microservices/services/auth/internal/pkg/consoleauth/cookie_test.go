package consoleauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTokenFromAuthorizationHeader(t *testing.T) {
	if got := TokenFromAuthorizationHeader("Bearer token-1"); got != "token-1" {
		t.Fatalf("token = %q, want token-1", got)
	}
	if got := TokenFromAuthorizationHeader("Basic token-1"); got != "" {
		t.Fatalf("basic auth token = %q, want empty", got)
	}
	if got := TokenFromAuthorizationHeader("Bearer "); got != "" {
		t.Fatalf("empty bearer token = %q, want empty", got)
	}
}

func TestTokenFromGinContextWithSourcePrefersBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer bearer-token")
	c.Request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-token"})

	token, source := TokenFromGinContextWithSource(c)
	if token != "bearer-token" || source != TokenSourceBearer {
		t.Fatalf("token/source = %q/%q, want bearer-token/%s", token, source, TokenSourceBearer)
	}
}

func TestTokenFromGinContextWithSourceFallsBackToCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: " cookie-token "})

	token, source := TokenFromGinContextWithSource(c)
	if token != "cookie-token" || source != TokenSourceCookie {
		t.Fatalf("token/source = %q/%q, want cookie-token/%s", token, source, TokenSourceCookie)
	}
}
