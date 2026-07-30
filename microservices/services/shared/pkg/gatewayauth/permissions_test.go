package gatewayauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireAnyPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		userID      string
		permissions string
		wantStatus  int
	}{
		{name: "missing identity", permissions: "crm:read", wantStatus: http.StatusUnauthorized},
		{name: "missing permission", userID: "7", permissions: "crm:read", wantStatus: http.StatusForbidden},
		{name: "matching permission", userID: "7", permissions: "crm:read,crm:write", wantStatus: http.StatusNoContent},
		{name: "super admin wildcard", userID: "7", permissions: "*", wantStatus: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/", RequireAnyPermission("crm:write"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(HeaderUserID, tt.userID)
			req.Header.Set(HeaderPermissions, tt.permissions)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}
