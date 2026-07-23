package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// OAuth2ServerAPI hosts the RFC 6749 protocol endpoints. token/introspect/
// revoke/userinfo return the BARE RFC JSON shape (third-party consumers);
// authorize returns the repo's wrapped envelope (consumed by our own frontend).
type OAuth2ServerAPI struct {
	server *authsvc.OAuth2ServerService
}

func NewOAuth2ServerAPI(server *authsvc.OAuth2ServerService) *OAuth2ServerAPI {
	return &OAuth2ServerAPI{server: server}
}

// rfcError writes an RFC 6749 §5.2 error body with the mapped HTTP status.
func rfcError(c *gin.Context, err *authsvc.OAuth2Error) {
	if err.Status == 401 {
		c.Header("WWW-Authenticate", `Basic realm="oauth2"`)
	}
	c.JSON(err.Status, gin.H{"error": err.Code, "error_description": err.Description})
}

func currentActor(c *gin.Context) (userID uint, username string, tenantID uint, ok bool) {
	uidVal, exists := c.Get("user_id")
	if !exists {
		return 0, "", 0, false
	}
	uid, _ := uidVal.(uint)
	uname, _ := c.Get("username")
	tid, _ := c.Get("tenant_id")
	username, _ = uname.(string)
	tenantID, _ = tid.(uint)
	if tenantID == 0 {
		tenantID = 1
	}
	return uid, username, tenantID, uid != 0
}

// GetAuthorize validates the request and returns the consent view (wrapped).
// Requires an authenticated console session (AuthMiddleware).
func (a *OAuth2ServerAPI) GetAuthorize(c *gin.Context) {
	userID, _, _, ok := currentActor(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	req := authsvc.AuthorizeRequest{
		ClientID:            c.Query("client_id"),
		RedirectURI:         c.Query("redirect_uri"),
		ResponseType:        c.Query("response_type"),
		Scope:               c.Query("scope"),
		State:               c.Query("state"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
		Nonce:               c.Query("nonce"),
	}
	view, oerr := a.server.ValidateAuthorizeRequest(c.Request.Context(), req, userID)
	if oerr != nil {
		response.BadRequest(c, oerr.Description)
		return
	}
	response.Success(c, view)
}

// PostAuthorize records consent and returns the redirect URL (wrapped). The
// frontend performs the actual browser navigation.
func (a *OAuth2ServerAPI) PostAuthorize(c *gin.Context) {
	userID, username, tenantID, ok := currentActor(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	var body struct {
		ClientID            string `json:"client_id"`
		RedirectURI         string `json:"redirect_uri"`
		ResponseType        string `json:"response_type"`
		Scope               string `json:"scope"`
		State               string `json:"state"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
		Nonce               string `json:"nonce"`
		Approved            bool   `json:"approved"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	req := authsvc.AuthorizeRequest{
		ClientID:            body.ClientID,
		RedirectURI:         body.RedirectURI,
		ResponseType:        body.ResponseType,
		Scope:               body.Scope,
		State:               body.State,
		CodeChallenge:       body.CodeChallenge,
		CodeChallengeMethod: body.CodeChallengeMethod,
		Nonce:               body.Nonce,
	}
	if !body.Approved {
		// User denied: hand back the standard access_denied redirect.
		denyURL, oerr := a.server.DenyRedirect(c.Request.Context(), req)
		if oerr != nil {
			response.BadRequest(c, oerr.Description)
			return
		}
		response.Success(c, gin.H{"redirect_url": denyURL})
		return
	}
	redirect, oerr := a.server.Approve(c.Request.Context(), userID, username, tenantID, req)
	if oerr != nil {
		response.BadRequest(c, oerr.Description)
		return
	}
	response.Success(c, gin.H{"redirect_url": redirect})
}

// clientCredentialsFromRequest extracts client_id/secret from HTTP Basic auth
// (preferred) or falling back to form fields.
func clientCredentialsFromRequest(c *gin.Context) (clientID, secret string) {
	if id, pw, ok := c.Request.BasicAuth(); ok {
		return id, pw
	}
	return c.PostForm("client_id"), c.PostForm("client_secret")
}

// PostToken is the RFC 6749 token endpoint (bare RFC responses).
func (a *OAuth2ServerAPI) PostToken(c *gin.Context) {
	grantType := c.PostForm("grant_type")
	clientID, secret := clientCredentialsFromRequest(c)
	ctx := c.Request.Context()

	client, oerr := a.server.AuthenticateClientContext(ctx, clientID, secret)
	if oerr != nil {
		rfcError(c, oerr)
		return
	}

	var (
		token *authsvc.TokenResponse
		terr  *authsvc.OAuth2Error
	)
	switch grantType {
	case "authorization_code":
		token, terr = a.server.ExchangeAuthorizationCode(ctx, client,
			c.PostForm("code"), c.PostForm("redirect_uri"), c.PostForm("code_verifier"))
	case "refresh_token":
		token, terr = a.server.ExchangeRefreshToken(ctx, client, c.PostForm("refresh_token"))
	case "client_credentials":
		token, terr = a.server.ClientCredentials(ctx, client, c.PostForm("scope"))
	default:
		rfcError(c, &authsvc.OAuth2Error{Code: "unsupported_grant_type", Description: "unsupported grant_type", Status: http.StatusBadRequest})
		return
	}
	if terr != nil {
		rfcError(c, terr)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, token)
}

// PostIntrospect implements RFC 7662 (bare RFC response).
func (a *OAuth2ServerAPI) PostIntrospect(c *gin.Context) {
	clientID, secret := clientCredentialsFromRequest(c)
	ctx := c.Request.Context()
	client, oerr := a.server.AuthenticateClientContext(ctx, clientID, secret)
	if oerr != nil {
		rfcError(c, oerr)
		return
	}
	token := c.PostForm("token")
	if token == "" {
		c.JSON(http.StatusOK, gin.H{"active": false})
		return
	}
	c.JSON(http.StatusOK, a.server.Introspect(ctx, client, token))
}

// PostRevoke implements RFC 7009 (always 200).
func (a *OAuth2ServerAPI) PostRevoke(c *gin.Context) {
	clientID, secret := clientCredentialsFromRequest(c)
	ctx := c.Request.Context()
	client, oerr := a.server.AuthenticateClientContext(ctx, clientID, secret)
	if oerr != nil {
		rfcError(c, oerr)
		return
	}
	if token := c.PostForm("token"); token != "" {
		a.server.Revoke(ctx, client, token, c.PostForm("token_type_hint"))
	}
	c.Status(http.StatusOK)
}

// GetUserInfo returns profile claims for the bearer access token (bare response).
func (a *OAuth2ServerAPI) GetUserInfo(c *gin.Context) {
	raw, _ := c.Get(middleware.OAuth2BearerContextKey)
	rawToken, _ := raw.(string)
	if rawToken == "" {
		c.Header("WWW-Authenticate", `Bearer realm="oauth2"`)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token", "error_description": "missing bearer token"})
		return
	}
	claims, oerr := a.server.UserInfo(c.Request.Context(), rawToken)
	if oerr != nil {
		c.Header("WWW-Authenticate", `Bearer realm="oauth2"`)
		c.JSON(oerr.Status, gin.H{"error": oerr.Code, "error_description": oerr.Description})
		return
	}
	c.JSON(http.StatusOK, claims)
}

// GetOpenIDConfiguration serves the OIDC discovery document (bare JSON, public).
func (a *OAuth2ServerAPI) GetOpenIDConfiguration(c *gin.Context) {
	oidc := a.server.OIDC()
	if oidc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "oidc_not_configured"})
		return
	}
	c.JSON(http.StatusOK, oidc.Discovery())
}

// GetJWKS serves the public JSON Web Key Set for id_token verification (bare JSON, public).
func (a *OAuth2ServerAPI) GetJWKS(c *gin.Context) {
	oidc := a.server.OIDC()
	if oidc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "oidc_not_configured"})
		return
	}
	jwks, err := oidc.JWKS(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, jwks)
}
