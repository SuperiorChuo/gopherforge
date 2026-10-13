package system

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/system/internal/pkg/weather"
)

// WeatherAPI 为仪表盘天气组件提供服务。该组件仅作装饰，因此
// 任何失败都降级返回空数据而非错误状态。
type WeatherAPI struct {
	service *weather.Service
}

func NewWeatherAPI() *WeatherAPI {
	return &WeatherAPI{service: weather.NewService(runtimeconfig.DefaultWeatherReader())}
}

// NewWeatherAPIWithService 从注入的服务创建 WeatherAPI。
func NewWeatherAPIWithService(service *weather.Service) *WeatherAPI {
	return &WeatherAPI{service: service}
}

// GetLiveWeather 处理 GET /api/v1/system/weather 请求。
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
