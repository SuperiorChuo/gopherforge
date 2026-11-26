// Package gatewayauth enforces identity and permissions asserted by the
// trusted ForwardAuth hop at the HTTP gateway.
package gatewayauth

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

const (
	HeaderUserID        = "X-Auth-User-ID"
	HeaderUsername      = "X-Auth-Username"
	HeaderTenantID      = "X-Auth-Tenant-ID"
	HeaderPlatformAdmin = "X-Auth-Platform-Admin"
	HeaderPermissions   = "X-Auth-Permissions"
	HeaderActTenantID   = "X-Act-Tenant-ID"

	// ContextKeyTrusted marks a Gin context populated from trusted ForwardAuth headers.
	ContextKeyTrusted = "gateway_auth_trusted"
)

// Identity carries the verified actor propagated by gateway ForwardAuth.
type Identity struct {
	UserID        uint
	Username      string
	TenantID      uint
	PlatformAdmin bool
}

// IdentityFromGinContext extracts a ForwardAuth-verified actor from request headers.
func IdentityFromGinContext(c *gin.Context) (Identity, bool) {
	rawUID := strings.TrimSpace(c.GetHeader(HeaderUserID))
	if rawUID == "" {
		return Identity{}, false
	}
	uid, err := strconv.ParseUint(rawUID, 10, 64)
	if err != nil || uid == 0 {
		return Identity{}, false
	}
	tenantID := uint(1)
	if rawTenant := strings.TrimSpace(c.GetHeader(HeaderTenantID)); rawTenant != "" {
		if tid, err := strconv.ParseUint(rawTenant, 10, 64); err == nil && tid > 0 {
			tenantID = uint(tid)
		}
	}
	rawPlatform := strings.TrimSpace(c.GetHeader(HeaderPlatformAdmin))
	platformAdmin := rawPlatform == "1" || strings.EqualFold(rawPlatform, "true")
	if platformAdmin {
		if rawAct := strings.TrimSpace(c.GetHeader(HeaderActTenantID)); rawAct != "" {
			if actID, err := strconv.ParseUint(rawAct, 10, 64); err == nil && actID > 0 {
				tenantID = uint(actID)
			}
		}
	}
	return Identity{
		UserID:        uint(uid),
		Username:      strings.TrimSpace(c.GetHeader(HeaderUsername)),
		TenantID:      tenantID,
		PlatformAdmin: platformAdmin,
	}, true
}

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

// HasAnyPermission evaluates required permissions against the ForwardAuth header.
func HasAnyPermission(header string, required ...string) (allowed bool, superAdmin bool) {
	granted := ParsePermissions(header)
	if _, ok := granted["*"]; ok {
		return true, true
	}
	if _, ok := granted["*:*:*"]; ok {
		return true, true
	}
	for _, code := range required {
		if _, ok := granted[code]; ok {
			return true, false
		}
	}
	return false, false
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
		if allowed, _ := HasAnyPermission(c.GetHeader(HeaderPermissions), required...); allowed {
			c.Next()
			return
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
