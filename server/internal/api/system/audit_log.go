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
		response.BadRequest(c, err.Error())
		return
	}

	result, err := a.logService.ListLogs(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, result)
}
