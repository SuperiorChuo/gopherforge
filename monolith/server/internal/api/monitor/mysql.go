package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/monitor"
)

type MySQLAPI struct {
	service *monitor.MySQLService
}

// NewMySQLAPIWithService creates a MySQLAPI instance from an injected service.
func NewMySQLAPIWithService(service *monitor.MySQLService) *MySQLAPI {
	return &MySQLAPI{service: service}
}

// GetMySQLInfo returns MySQL information.
func (a *MySQLAPI) GetMySQLInfo(c *gin.Context) {
	data, err := a.service.GetMySQLInfoContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get MySQL info", err)
		return
	}
	response.Success(c, data)
}
