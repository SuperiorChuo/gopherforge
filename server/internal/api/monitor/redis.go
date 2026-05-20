package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/monitor"
)

type RedisAPI struct {
	service *monitor.RedisService
}

func NewRedisAPI() *RedisAPI {
	return &RedisAPI{
		service: monitor.NewRedisService(),
	}
}

// GetRedisInfo 获取 Redis 信息
func (a *RedisAPI) GetRedisInfo(c *gin.Context) {
	data, err := a.service.GetRedisInfo()
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	response.Success(c, data)
}
