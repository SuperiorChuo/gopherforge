package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
)

func TestSetConsoleSessionCookieUsesSecureFlagInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldConfig := config.Cfg
	config.Cfg = config.Config{App: config.AppCfg{Env: "production"}}
	t.Cleanup(func() {
		config.Cfg = oldConfig
	})

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		setConsoleSessionCookie(c, "token-value", 60)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Name != consoleauth.SessionCookieName {
		t.Fatalf("cookie name = %q, want %q", cookies[0].Name, consoleauth.SessionCookieName)
	}
	if !cookies[0].Secure {
		t.Fatal("console session cookie should be secure in production")
	}
}
