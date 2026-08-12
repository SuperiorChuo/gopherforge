package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/response"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
)

type edgeCertificateExportStepUpVerifier interface {
	VerifyAndIssueContext(context.Context, uint, string, authsvc.EdgeCertificateExportStepUpRequest) (*authsvc.EdgeCertificateExportStepUpResponse, error)
}

// EdgeCertificateExportStepUpAPI handles fresh-factor verification before a
// private-key export. The returned proof is opaque, short-lived and single-use.
type EdgeCertificateExportStepUpAPI struct {
	service edgeCertificateExportStepUpVerifier
}

func newEdgeCertificateExportStepUpAPIFromDeps(deps sharedapi.Dependencies) *EdgeCertificateExportStepUpAPI {
	return &EdgeCertificateExportStepUpAPI{
		service: authsvc.NewEdgeCertificateExportStepUpServiceWithDeps(deps.DB, deps.Redis),
	}
}

func edgeCertificateExportNoStoreHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		setEdgeCertificateExportNoStoreHeaders(c)
		c.Next()
	}
}

func setEdgeCertificateExportNoStoreHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Vary", "Authorization, Cookie")
	c.Header("X-Content-Type-Options", "nosniff")
}

// IssueEdgeCertificateExportProof verifies the current password and mandatory
// TOTP before binding a proof to exactly one certificate export and session.
func (a *EdgeCertificateExportStepUpAPI) IssueEdgeCertificateExportProof(c *gin.Context) {
	setEdgeCertificateExportNoStoreHeaders(c)

	userValue, exists := c.Get("user_id")
	userID, ok := userValue.(uint)
	if !exists || !ok || userID == 0 {
		response.Unauthorized(c, "authentication required")
		return
	}
	sessionValue, exists := c.Get("session_id")
	sessionID, ok := sessionValue.(string)
	if !exists || !ok || sessionID == "" {
		response.Unauthorized(c, "authentication required")
		return
	}

	var req authsvc.EdgeCertificateExportStepUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}
	if a == nil || a.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "step-up verification unavailable")
		return
	}

	result, err := a.service.VerifyAndIssueContext(c.Request.Context(), userID, sessionID, req)
	if err != nil {
		switch {
		case errors.Is(err, authsvc.ErrStepUpVerificationFailed):
			// One response covers bad password, missing/invalid TOTP, disabled
			// users and freshly revoked platform-admin status. This is an
			// authorization failure, not an expired bearer token: returning 401
			// would make the web interceptor refresh/logout a valid session.
			response.Forbidden(c, "step-up verification failed")
		case errors.Is(err, authsvc.ErrStepUpUnavailable):
			response.Error(c, http.StatusServiceUnavailable, "step-up verification unavailable")
		case errors.Is(err, authsvc.ErrStepUpRateLimited):
			c.Header("Retry-After", "300")
			response.Error(c, http.StatusTooManyRequests, "too many step-up attempts")
		default:
			internalServerError(c, "failed to issue edge certificate export proof", err)
		}
		return
	}
	response.Success(c, result)
}
