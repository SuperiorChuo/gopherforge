package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/events"
)

// Event publishing is best-effort: the default publisher is nil-safe, so these
// helpers never fail or block the request path.

func publishLoginSuccess(c *gin.Context, userID uint, username, loginType string, tenantID uint) {
	if tenantID == 0 {
		tenantID = 1
	}
	events.Default().PublishLoginSuccess(events.LoginSuccessEvent{
		UserID:    userID,
		Username:  username,
		TenantID:  tenantID,
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		LoginType: loginType,
	})
}

func publishLoginFailed(c *gin.Context, username, reason string, tenantID uint) {
	if tenantID == 0 {
		tenantID = 1
	}
	events.Default().PublishLoginFailed(events.LoginFailedEvent{
		Username:  username,
		TenantID:  tenantID,
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Reason:    reason,
	})
}

func publishLogout(c *gin.Context, userID uint, username string) {
	events.Default().PublishLogout(events.LogoutEvent{
		UserID:   userID,
		Username: username,
		IP:       c.ClientIP(),
	})
}
