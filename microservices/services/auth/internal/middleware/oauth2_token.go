package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// OAuth2BearerContextKey stores the raw OAuth2 access token extracted from the
// Authorization header, for resource endpoints (e.g. /oauth2/userinfo).
const OAuth2BearerContextKey = "oauth2_bearer"

// OAuth2BearerMiddleware extracts a Bearer token into the context WITHOUT
// validating it — validation (hash lookup, expiry, revocation) is done by the
// service so it can return the RFC-shaped 401. This keeps the opaque-token
// resource path separate from the console JWT AuthMiddleware.
func OAuth2BearerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			c.Set(OAuth2BearerContextKey, strings.TrimSpace(parts[1]))
		}
		c.Next()
	}
}
