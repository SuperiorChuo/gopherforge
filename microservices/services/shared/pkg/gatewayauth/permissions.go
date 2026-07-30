// Package gatewayauth enforces identity and permissions asserted by the
// trusted ForwardAuth hop at the HTTP gateway.
package gatewayauth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

const (
	HeaderUserID      = "X-Auth-User-ID"
	HeaderPermissions = "X-Auth-Permissions"
)

// ParsePermissions returns a set of permission codes from the trusted header.
func ParsePermissions(header string) map[string]struct{} {
	permissions := make(map[string]struct{})
	for _, code := range strings.Split(header, ",") {
		code = strings.TrimSpace(code)
		if code != "" {
			permissions[code] = struct{}{}
		}
	}
	return permissions
}

// RequireAnyPermission allows a request with any required permission. The
// wildcard is emitted only for super_admin by auth-service.
func RequireAnyPermission(required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader(HeaderUserID)) == "" {
			response.Unauthorized(c, "authentication required")
			c.Abort()
			return
		}
		granted := ParsePermissions(c.GetHeader(HeaderPermissions))
		if _, ok := granted["*"]; ok {
			c.Next()
			return
		}
		for _, code := range required {
			if _, ok := granted[code]; ok {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, "insufficient permissions")
		c.Abort()
	}
}

// RequireReadWritePermissions selects a permission by HTTP semantics. Read
// methods require readPermission; state-changing methods require writePermission.
func RequireReadWritePermissions(readPermission, writePermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		required := writePermission
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			required = readPermission
		}
		RequireAnyPermission(required)(c)
	}
}
