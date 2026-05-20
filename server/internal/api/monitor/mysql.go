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

// GetMySQLInfo 获取 MySQL 信息
func (a *MySQLAPI) GetMySQLInfo(c *gin.Context) {
	data, err := a.service.GetMySQLInfo()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, data)
}
