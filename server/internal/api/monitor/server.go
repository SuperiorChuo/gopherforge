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

// GetServerInfo 获取服务器信息
func (a *ServerAPI) GetServerInfo(c *gin.Context) {
	data, err := a.service.GetServerInfo()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, data)
}
