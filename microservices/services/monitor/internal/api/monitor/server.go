package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/monitor"
)

type ServerAPI struct {
	service *monitor.ServerService
}

func NewServerAPI() *ServerAPI {
	return &ServerAPI{
		service: monitor.NewServerService(),
	}
}

// GetServerInfo returns server information.
func (a *ServerAPI) GetServerInfo(c *gin.Context) {
	data, err := a.service.GetServerInfoContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get server info", err)
		return
	}
	response.Success(c, data)
}
