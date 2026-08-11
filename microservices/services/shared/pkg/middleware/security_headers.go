package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders applies conservative browser security headers.
func SecurityHeaders(enabled bool, hsts bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled {
			c.Header("X-Content-Type-Options", "nosniff")
			c.Header("X-Frame-Options", "DENY")
			c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			c.Header("X-XSS-Protection", "0")
			if hsts {
				c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
		}
		c.Next()
	}
}
