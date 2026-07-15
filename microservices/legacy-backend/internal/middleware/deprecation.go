package middleware

import "github.com/gin-gonic/gin"

const defaultSunsetAt = "Wed, 31 Dec 2026 23:59:59 GMT"

// DeprecatedRoute marks a compatibility route while preserving its handler behavior.
func DeprecatedRoute(successor string) gin.HandlerFunc {
	return DeprecatedRouteWithSunset(successor, defaultSunsetAt)
}

func DeprecatedRouteWithSunset(successor string, sunsetAt string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		if sunsetAt != "" {
			c.Header("Sunset", sunsetAt)
		}
		if successor != "" {
			c.Header("Link", "<"+successor+">; rel=\"successor-version\"")
		}
		c.Next()
	}
}
