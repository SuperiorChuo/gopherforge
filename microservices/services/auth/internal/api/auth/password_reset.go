package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
)

// forgotPasswordLimit throttles link issuance per IP+email (10 per 10 min).
var forgotPasswordLimit = middleware.RateLimit(middleware.RateLimitConfig{
	Window:      10 * time.Minute,
	MaxRequests: 10,
	KeyPrefix:   "forgot_pwd",
})

type PasswordResetAPI struct {
	service *authsvc.PasswordResetService
}

func NewPasswordResetAPI(db *gorm.DB) *PasswordResetAPI {
	return &PasswordResetAPI{service: authsvc.NewPasswordResetService(db)}
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ForgotPassword answers a reset email; always 200 for unknown email to avoid
// account enumeration. Only the rate limit rejects.
func (a *PasswordResetAPI) ForgotPassword(c *gin.Context) {
	if a == nil || a.service == nil {
		response.Success(c, gin.H{"message": "if the email exists, a reset link has been sent"})
		return
	}
	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email is required and must be valid")
		return
	}
	if err := a.service.ForgotPasswordContext(c.Request.Context(), strings.ToLower(strings.TrimSpace(req.Email))); err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to send reset email")
		return
	}
	response.Success(c, gin.H{"message": "if the email exists, a reset link has been sent"})
}

// ResetPassword consumes the token and sets the new password.
func (a *PasswordResetAPI) ResetPassword(c *gin.Context) {
	if a == nil || a.service == nil {
		response.BadRequest(c, "reset service unavailable")
		return
	}
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "token and new_password are required")
		return
	}
	if err := a.service.ResetPasswordContext(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		response.BadRequest(c, "reset token is invalid or expired")
		return
	}
	response.Success(c, gin.H{"message": "password has been reset, please sign in"})
}

// RegisterPasswordResetPublicRoutes mounts the two public forgot/reset
// endpoints under /api/v1/password/ (routed by the gateway).
func RegisterPasswordResetPublicRoutes(r gin.IRoutes, deps sharedapi.Dependencies) {
	api := NewPasswordResetAPI(deps.DB)
	r.POST("/password/forgot", forgotPasswordLimit, api.ForgotPassword)
	r.POST("/password/reset", api.ResetPassword)
}
