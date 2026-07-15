package consoleauth

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const SessionCookieName = "black8console_session"

const (
	TokenSourceBearer = "bearer"
	TokenSourceCookie = "cookie"
)

func TokenFromAuthorizationHeader(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) || len(header) <= len(prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func TokenFromGinContext(c *gin.Context) string {
	token, _ := TokenFromGinContextWithSource(c)
	return token
}

func TokenFromGinContextWithSource(c *gin.Context) (string, string) {
	if token := TokenFromAuthorizationHeader(c.GetHeader("Authorization")); token != "" {
		return token, TokenSourceBearer
	}
	if token, err := c.Cookie(SessionCookieName); err == nil {
		if trimmed := strings.TrimSpace(token); trimmed != "" {
			return trimmed, TokenSourceCookie
		}
	}
	return "", ""
}
