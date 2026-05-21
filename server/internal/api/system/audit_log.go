package system

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	service "github.com/go-admin-kit/server/internal/service/system"
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
