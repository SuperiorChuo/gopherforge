package monitor

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	monitorsvc "github.com/go-admin-kit/services/monitor/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"gorm.io/gorm"
)

type TaskRunAPI struct {
	service *monitorsvc.TaskRunService
}

type taskRunListRequest struct {
	pagination.PageRequest
	Keyword   string `form:"keyword"`
	Service   string `form:"service"`
	Status    string `form:"status"`
	Source    string `form:"source"`
	StartTime string `form:"start_time"`
	EndTime   string `form:"end_time"`
}

func (a *TaskRunAPI) List(c *gin.Context) {
	var req taskRunListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, invalidQueryParametersMessage)
		return
	}
	startAt, ok := parseTaskRunTime(c, req.StartTime)
	if !ok {
		return
	}
	endAt, ok := parseTaskRunTime(c, req.EndTime)
	if !ok {
		return
	}
	query := monitorsvc.TaskRunQuery{
		PageRequest: req.PageRequest,
		Keyword:     strings.TrimSpace(req.Keyword), Service: strings.TrimSpace(req.Service),
		Status: strings.TrimSpace(req.Status), Source: strings.TrimSpace(req.Source),
		StartAt: startAt, EndAt: endAt,
	}
	list, total, err := a.service.ListContext(c.Request.Context(), query)
	if err != nil {
		writeTaskRunError(c, err)
		return
	}
	page, pageSize := query.Page, query.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > monitorsvc.MaxTaskRunPageSize {
		pageSize = monitorsvc.MaxTaskRunPageSize
	}
	response.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (a *TaskRunAPI) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid task run id")
		return
	}
	run, err := a.service.GetContext(c.Request.Context(), id)
	if err != nil {
		writeTaskRunError(c, err)
		return
	}
	response.Success(c, run)
}

func (a *TaskRunAPI) Summary(c *gin.Context) {
	windowHours := 0
	if raw := c.Query("window_hours"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			response.BadRequest(c, "window_hours must be a number")
			return
		}
		windowHours = parsed
	}
	summary, err := a.service.SummaryContext(c.Request.Context(), windowHours)
	if err != nil {
		writeTaskRunError(c, err)
		return
	}
	response.Success(c, summary)
}

func parseTaskRunTime(c *gin.Context, raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		response.BadRequest(c, "time filters must use RFC3339")
		return nil, false
	}
	return &parsed, true
}

func writeTaskRunError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "task run not found")
	case errors.Is(err, monitorsvc.ErrInvalidTaskRunStatus):
		response.BadRequest(c, monitorsvc.ErrInvalidTaskRunStatus.Error())
	case errors.Is(err, monitorsvc.ErrInvalidTaskRunSource):
		response.BadRequest(c, monitorsvc.ErrInvalidTaskRunSource.Error())
	case errors.Is(err, monitorsvc.ErrInvalidTaskRunWindow):
		response.BadRequest(c, monitorsvc.ErrInvalidTaskRunWindow.Error())
	case errors.Is(err, monitorsvc.ErrInvalidTaskRunRange):
		response.BadRequest(c, monitorsvc.ErrInvalidTaskRunRange.Error())
	default:
		internalServerError(c, "failed to query task runs", err)
	}
}
