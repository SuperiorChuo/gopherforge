package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeprecatedRouteAddsStandardHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/legacy", DeprecatedRoute("/api/v1/users"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/legacy", nil)
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Deprecation"); got != "true" {
		t.Fatalf("Deprecation header = %q, want true", got)
	}
	if got := recorder.Header().Get("Sunset"); got != defaultSunsetAt {
		t.Fatalf("Sunset header = %q, want %q", got, defaultSunsetAt)
	}
	wantLink := `</api/v1/users>; rel="successor-version"`
	if got := recorder.Header().Get("Link"); got != wantLink {
		t.Fatalf("Link header = %q, want %q", got, wantLink)
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
