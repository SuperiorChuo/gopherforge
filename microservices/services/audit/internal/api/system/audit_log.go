package system

import (
	"github.com/gin-gonic/gin"
	service "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// AuditLogAPI exposes independent business audit logs.
type AuditLogAPI struct {
	logService service.AuditLogService
}

func NewAuditLogAPI() *AuditLogAPI {
	return &AuditLogAPI{
		logService: service.AuditLogService{},
	}
}

// NewAuditLogAPIWithService creates an AuditLogAPI instance from an injected service.
func NewAuditLogAPIWithService(logService service.AuditLogService) *AuditLogAPI {
	return &AuditLogAPI{logService: logService}
}

func (a *AuditLogAPI) GetAuditLogs(c *gin.Context) {
	var req service.AuditLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	result, err := a.logService.ListLogsContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to list audit logs", err)
		return
	}

	response.Success(c, result)
}
