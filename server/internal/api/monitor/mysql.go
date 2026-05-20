package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/monitor"
)

type MySQLAPI struct {
	service *monitor.MySQLService
}

func NewMySQLAPI() *MySQLAPI {
	return &MySQLAPI{
		service: monitor.NewMySQLService(),
	}
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
