package system

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/system/internal/pkg/weather"
)

// WeatherAPI serves the dashboard weather chip. The chip is decorative, so
// every failure degrades to an empty payload instead of an error status.
type WeatherAPI struct {
	service *weather.Service
}

func NewWeatherAPI() *WeatherAPI {
	return &WeatherAPI{service: weather.NewService(runtimeconfig.DefaultWeatherReader())}
}

// NewWeatherAPIWithService creates a WeatherAPI from an injected service.
func NewWeatherAPIWithService(service *weather.Service) *WeatherAPI {
	return &WeatherAPI{service: service}
}

// GetLiveWeather handles GET /api/v1/system/weather.
// 未配置/上游失败一律 200 + data:null：错误处理中间件会给 404 追加兜底 body
// 造成双 JSON，且天气是装饰性信息，前端对 null 静默隐藏即可。
func (a *WeatherAPI) GetLiveWeather(c *gin.Context) {
	live, err := a.service.LiveByIP(c.Request.Context(), c.ClientIP())
	if err != nil {
		response.Success(c, nil)
		return
	}
	response.Success(c, live)
}
