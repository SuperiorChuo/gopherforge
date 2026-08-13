package system

import (
	"errors"

	"github.com/gin-gonic/gin"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// PermissionDiagnosticAPI explains effective authorization decisions.
type PermissionDiagnosticAPI struct {
	service systemsvc.PermissionDiagnosticService
}

// NewPermissionDiagnosticAPIWithService creates an API with an injected service.
func NewPermissionDiagnosticAPIWithService(service systemsvc.PermissionDiagnosticService) *PermissionDiagnosticAPI {
	return &PermissionDiagnosticAPI{service: service}
}

// DiagnosePermission returns the user's role, permission, package, and data-scope chain.
func (a *PermissionDiagnosticAPI) DiagnosePermission(c *gin.Context) {
	var req systemsvc.PermissionDiagnosticRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	result, err := a.service.DiagnoseContext(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, systemsvc.ErrPermissionDiagnosticUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		internalServerError(c, "failed to diagnose permission", err)
		return
	}
	response.Success(c, result)
}
