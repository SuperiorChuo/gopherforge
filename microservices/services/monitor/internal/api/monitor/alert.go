package monitor

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	monitorsvc "github.com/go-admin-kit/services/monitor/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
)

type AlertAPI struct {
	service *monitorsvc.AlertService
}

type saveAlertRuleRequest struct {
	Name            string     `json:"name"`
	Metric          string     `json:"metric"`
	Operator        string     `json:"operator"`
	Threshold       *float64   `json:"threshold"`
	DurationSeconds *int64     `json:"duration_seconds"`
	Severity        string     `json:"severity"`
	Enabled         *bool      `json:"enabled"`
	NotifyOnResolve *bool      `json:"notify_on_resolve"`
	NotifyChannels  []string   `json:"notify_channels"`
	SilenceUntil    *time.Time `json:"silence_until"`
}

func NewAlertAPIWithService(service *monitorsvc.AlertService) *AlertAPI {
	return &AlertAPI{service: service}
}

func (a *AlertAPI) GetMetrics(c *gin.Context) {
	response.Success(c, gin.H{"list": monitorsvc.ListAlertMetrics()})
}

func (a *AlertAPI) GetRules(c *gin.Context) {
	req := pagination.GetPageRequest(c)
	filter := monitordao.AlertRuleFilter{
		Name:   strings.TrimSpace(c.Query("name")),
		Metric: strings.TrimSpace(c.Query("metric")),
		State:  strings.ToLower(strings.TrimSpace(c.Query("state"))),
	}
	if err := monitorsvc.ValidateAlertState(filter.State); err != nil {
		response.BadRequest(c, "invalid alert state")
		return
	}
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "enabled must be a boolean")
			return
		}
		filter.Enabled = &enabled
	}
	rules, total, err := a.service.ListRulesContext(c.Request.Context(), req, filter)
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.PageSuccess(c, rules, total, req.Page, req.PageSize)
}

func (a *AlertAPI) GetSummary(c *gin.Context) {
	summary, err := a.service.GetRuleSummaryContext(c.Request.Context())
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, summary)
}

func (a *AlertAPI) CreateRule(c *gin.Context) {
	request, ok := bindAlertRuleRequest(c)
	if !ok {
		return
	}
	rule, err := a.service.CreateRuleContext(c.Request.Context(), request)
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, rule)
}

func (a *AlertAPI) UpdateRule(c *gin.Context) {
	id, ok := parseAlertRuleID(c)
	if !ok {
		return
	}
	request, ok := bindAlertRuleRequest(c)
	if !ok {
		return
	}
	rule, err := a.service.UpdateRuleContext(c.Request.Context(), id, request)
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, rule)
}

func (a *AlertAPI) DeleteRule(c *gin.Context) {
	id, ok := parseAlertRuleID(c)
	if !ok {
		return
	}
	if err := a.service.DeleteRuleContext(c.Request.Context(), id); err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, nil)
}

func (a *AlertAPI) EvaluateRule(c *gin.Context) {
	id, ok := parseAlertRuleID(c)
	if !ok {
		return
	}
	rule, event, err := a.service.EvaluateRuleContext(c.Request.Context(), id)
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, gin.H{"rule": rule, "event": event})
}

func (a *AlertAPI) GetEvents(c *gin.Context) {
	req := pagination.GetPageRequest(c)
	filter := monitordao.AlertEventFilter{
		RuleName:     strings.TrimSpace(c.Query("rule_name")),
		Status:       strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Severity:     strings.ToLower(strings.TrimSpace(c.Query("severity"))),
		NotifyStatus: strings.ToLower(strings.TrimSpace(c.Query("notify_status"))),
	}
	if raw := strings.TrimSpace(c.Query("rule_id")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			response.BadRequest(c, "rule_id must be a positive integer")
			return
		}
		filter.RuleID = uint(id)
	}
	for _, validation := range []error{
		monitorsvc.ValidateAlertEventStatus(filter.Status),
		monitorsvc.ValidateAlertSeverity(filter.Severity),
		monitorsvc.ValidateAlertNotifyStatus(filter.NotifyStatus),
	} {
		if validation != nil {
			response.BadRequest(c, "invalid alert event filters")
			return
		}
	}
	events, total, err := a.service.ListEventsContext(c.Request.Context(), req, filter)
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.PageSuccess(c, events, total, req.Page, req.PageSize)
}

func bindAlertRuleRequest(c *gin.Context) (monitorsvc.AlertRuleInput, bool) {
	var request saveAlertRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return monitorsvc.AlertRuleInput{}, false
	}
	if request.Threshold == nil {
		response.BadRequest(c, "threshold is required")
		return monitorsvc.AlertRuleInput{}, false
	}
	duration := int64(0)
	if request.DurationSeconds != nil {
		duration = *request.DurationSeconds
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	notifyOnResolve := true
	if request.NotifyOnResolve != nil {
		notifyOnResolve = *request.NotifyOnResolve
	}
	return monitorsvc.AlertRuleInput{
		Name:            request.Name,
		Metric:          request.Metric,
		Operator:        request.Operator,
		Threshold:       *request.Threshold,
		DurationSeconds: duration,
		Severity:        request.Severity,
		Enabled:         enabled,
		NotifyOnResolve: notifyOnResolve,
		NotifyChannels:  append(localmodel.NotifyChannelList(nil), request.NotifyChannels...),
		SilenceUntil:    request.SilenceUntil,
	}, true
}

func parseAlertRuleID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid alert rule id")
		return 0, false
	}
	return uint(id), true
}

func (a *AlertAPI) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, monitorsvc.ErrInvalidAlertRuleName),
		errors.Is(err, monitorsvc.ErrInvalidAlertMetric),
		errors.Is(err, monitorsvc.ErrInvalidAlertOperator),
		errors.Is(err, monitorsvc.ErrInvalidAlertThreshold),
		errors.Is(err, monitorsvc.ErrInvalidAlertDuration),
		errors.Is(err, monitorsvc.ErrInvalidAlertSeverity),
		errors.Is(err, monitorsvc.ErrAlertRuleDisabled):
		response.BadRequest(c, "invalid alert rule")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "alert rule not found")
	case errors.Is(err, monitorsvc.ErrAlertMetricUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "alert metric collection failed")
	default:
		internalServerError(c, "failed to process alert request", err)
	}
}
