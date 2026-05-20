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

// GetRedisInfo returns Redis information.
func (a *RedisAPI) GetRedisInfo(c *gin.Context) {
	data, err := a.service.GetRedisInfoContext(c.Request.Context())
	if err != nil {
		internalServerError(c, "failed to get Redis info", err)
		return
	}
	response.Success(c, data)
}
