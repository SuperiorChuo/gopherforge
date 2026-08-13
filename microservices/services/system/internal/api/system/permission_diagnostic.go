package system

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

// PermissionMenuDiagnosticAPI explains menu bindings for a permission resource.
type PermissionMenuDiagnosticAPI struct {
	service systemsvc.PermissionMenuDiagnosticService
}

func NewPermissionMenuDiagnosticAPIWithService(service systemsvc.PermissionMenuDiagnosticService) *PermissionMenuDiagnosticAPI {
	return &PermissionMenuDiagnosticAPI{service: service}
}

func (a *PermissionMenuDiagnosticAPI) DiagnoseMenus(c *gin.Context) {
	permission := strings.TrimSpace(c.Query("permission"))
	if permission == "" {
		response.BadRequest(c, "permission is required")
		return
	}
	result, err := a.service.DiagnoseContext(c.Request.Context(), permission)
	if err != nil {
		internalServerError(c, "failed to diagnose permission menus", err)
		return
	}
	response.Success(c, result)
}
