package monitor

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type RedisAPI struct {
	service *monitor.RedisService
}

func NewRedisAPI() *RedisAPI {
	return &RedisAPI{
		service: monitor.NewRedisService(),
	}
}

// NewRedisAPIWithService creates a RedisAPI instance from an injected service.
func NewRedisAPIWithService(service *monitor.RedisService) *RedisAPI {
	return &RedisAPI{service: service}
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
