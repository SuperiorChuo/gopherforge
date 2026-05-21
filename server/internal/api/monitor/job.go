package monitor

import (
	"errors"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/monitor"
	"gorm.io/gorm"
)

type JobAPI struct {
	service *monitor.JobService
}

type cleanupJobLogsRequest struct {
	RetentionDays *int `json:"retention_days"`
}

const (
	invalidRequestBodyMessage     = "invalid request body"
	invalidQueryParametersMessage = "invalid query parameters"
)

func NewJobAPI() *JobAPI {
	return &JobAPI{
		service: monitor.GetJobService(),
	}
}

// GetJobList returns paginated scheduled jobs.
func (a *JobAPI) GetJobList(c *gin.Context) {
	var req pagination.PageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, invalidQueryParametersMessage)
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	name := c.Query("name")
	var status *int8
	if s := c.Query("status"); s != "" {
		st, err := strconv.Atoi(s)
		if err != nil || (st != 0 && st != 1) {
			response.BadRequest(c, "status must be 0 or 1")
			return
		}
		st8 := int8(st)
		status = &st8
	}

	jobs, total, err := a.service.GetJobListContext(c.Request.Context(), req, name, status)
	if err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, gin.H{
		"list":      jobs,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// CreateJob creates a scheduled job.
func (a *JobAPI) CreateJob(c *gin.Context) {
	var job model.ScheduledJob
	if err := c.ShouldBindJSON(&job); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}

	if err := a.service.CreateJobContext(c.Request.Context(), &job); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, job)
}

// UpdateJob updates a scheduled job.
func (a *JobAPI) UpdateJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	var job model.ScheduledJob
	if err := c.ShouldBindJSON(&job); err != nil {
		response.BadRequest(c, invalidRequestBodyMessage)
		return
	}
	job.ID = id

	if err := a.service.UpdateJobContext(c.Request.Context(), &job); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, job)
}

// DeleteJob deletes a scheduled job.
func (a *JobAPI) DeleteJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.DeleteJobContext(c.Request.Context(), id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// StartJob starts a scheduled job.
func (a *JobAPI) StartJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.StartJobByIDContext(c.Request.Context(), id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// StopJob stops a scheduled job.
func (a *JobAPI) StopJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.StopJobByIDContext(c.Request.Context(), id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// RunJob runs a scheduled job immediately.
func (a *JobAPI) RunJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.RunJobContext(c.Request.Context(), id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// GetJobHealth returns scheduled job health.
func (a *JobAPI) GetJobHealth(c *gin.Context) {
	windowHours := 0
	if v := c.Query("window_hours"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "window_hours must be greater than 0")
			return
		}
		windowHours = parsed
	}

	health, err := a.service.CheckJobHealthContext(c.Request.Context(), windowHours)
	if err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, health)
}

// CleanupJobLogs removes job logs older than the retention window.
func (a *JobAPI) CleanupJobLogs(c *gin.Context) {
	retentionDays := monitor.DefaultJobLogRetentionDays

	var req cleanupJobLogsRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(c, invalidRequestBodyMessage)
			return
		}
		if req.RetentionDays != nil {
			retentionDays = *req.RetentionDays
		}
	}

	if v := c.Query("retention_days"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(c, "retention_days must be a number")
			return
		}
		retentionDays = parsed
	}

	result, err := a.service.CleanupJobLogsContext(c.Request.Context(), retentionDays)
	if err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, result)
}

func parseJobID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid job id")
		return 0, false
	}
	return uint(id), true
}

func (a *JobAPI) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, monitor.ErrInvalidCronExpression), errors.Is(err, monitor.ErrInvalidRetentionDays):
		response.BadRequest(c, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "job not found")
	default:
		internalServerError(c, "failed to process scheduled job request", err)
	}
}
