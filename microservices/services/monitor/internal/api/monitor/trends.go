package monitor

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	monitorsvc "github.com/go-admin-kit/services/monitor/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type TrendsAPI struct {
	service *monitorsvc.MetricTrendService
}

func NewTrendsAPIWithService(service *monitorsvc.MetricTrendService) *TrendsAPI {
	return &TrendsAPI{service: service}
}

func (a *TrendsAPI) GetTrends(c *gin.Context) {
	metric := strings.TrimSpace(c.Query("metric"))
	rng := strings.TrimSpace(c.Query("range"))
	if metric == "" {
		response.BadRequest(c, "metric is required")
		return
	}
	trend, err := a.service.QueryTrendContext(c.Request.Context(), metric, rng)
	if err != nil {
		if errors.Is(err, monitorsvc.ErrInvalidTrendRange) || errors.Is(err, monitorsvc.ErrInvalidAlertMetric) {
			response.BadRequest(c, "invalid metric or trend range")
			return
		}
		response.Error(c, http.StatusInternalServerError, "failed to query metric trend")
		return
	}
	response.Success(c, trend)
}
