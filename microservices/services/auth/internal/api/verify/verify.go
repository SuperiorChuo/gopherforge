// Package verify implements the Traefik forwardAuth endpoint. Traefik calls
// GET /internal/verify for gateway-routed requests and copies the returned
// X-Auth-* headers upstream.
package verify

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/pkg/consoleauth"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/auth/internal/pkg/response"
)

// Response headers copied upstream by Traefik on successful verification.
const (
	HeaderUserID   = "X-Auth-User-ID"
	HeaderUsername = "X-Auth-Username"
	HeaderTenantID = "X-Auth-Tenant-ID"
)

// Handler verifies bearer tokens and console session cookies with the exact
// semantics of middleware.AuthMiddleware, except that a request carrying no
// credentials at all passes through anonymously (the monolith still enforces
// authentication on its protected routes).
type Handler struct {
	consoleSessions middleware.ConsoleSessionValidator
	users           middleware.AuthUserStore
}

// NewHandler creates a forwardAuth verification handler.
func NewHandler(consoleSessions middleware.ConsoleSessionValidator, users middleware.AuthUserStore) *Handler {
	return &Handler{consoleSessions: consoleSessions, users: users}
}

// Verify handles GET /internal/verify.
func (h *Handler) Verify(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	tokenString, tokenSource := consoleauth.TokenFromGinContextWithSource(c)

	// Anonymous pass-through: no bearer token and no console session cookie.
	if authHeader == "" && tokenString == "" {
		c.Status(http.StatusOK)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if authHeader != "" && (len(parts) != 2 || parts[0] != "Bearer") {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthHeaderInvalid, "Authorization header format must be Bearer {token}")
		return
	}

	claims, err := jwt.ParseTokenContext(c.Request.Context(), tokenString)
	if err != nil {
		var message string
		errorCode := response.ErrorCodeAuthTokenInvalid
		switch err {
		case jwt.ErrExpiredToken:
			message = "Token has expired"
			errorCode = response.ErrorCodeAuthTokenExpired
		case jwt.ErrInvalidToken:
			message = "Invalid token"
		case jwt.ErrRevokedToken:
			message = "Token has been revoked"
			errorCode = response.ErrorCodeAuthTokenRevoked
		default:
			message = "Unauthorized"
		}
		response.UnauthorizedWithCode(c, errorCode, message)
		return
	}
	if claims.TokenType != jwt.AccessTokenType {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthTokenInvalid, "Invalid token type")
		return
	}
	if tokenSource == consoleauth.TokenSourceCookie {
		if h == nil || h.consoleSessions == nil || h.users == nil {
			response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
			return
		}
		if _, err := h.consoleSessions.ValidateActiveSessionContext(c.Request.Context(), claims.ID, claims.Username); err != nil {
			response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
			return
		}
		user, err := h.users.GetUserWithRolesContext(c.Request.Context(), claims.UserID)
		if err != nil || user.Status != 1 {
			response.UnauthorizedWithCode(c, response.ErrorCodeConsoleLoginRequired, "Console login required")
			return
		}
	}

	c.Header(HeaderUserID, strconv.FormatUint(uint64(claims.UserID), 10))
	c.Header(HeaderUsername, claims.Username)
	c.Header(HeaderTenantID, strconv.FormatUint(uint64(jwt.NormalizeTenantID(claims.TenantID)), 10))
	c.Status(http.StatusOK)
}
