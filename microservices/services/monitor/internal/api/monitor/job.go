package monitor

import (
	"errors"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/monitor/internal/service/monitor"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
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
	var job localmodel.ScheduledJob
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

	var job localmodel.ScheduledJob
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

// GetJobTargets returns the built-in invoke targets the console may pick from.
// Same source as the executor's dispatch table, so the dropdown can never drift
// from what the scheduler actually knows how to run.
func (a *JobAPI) GetJobTargets(c *gin.Context) {
	response.Success(c, gin.H{"list": monitor.ListJobTargets()})
}

// GetJobLogList returns paginated scheduled-job execution logs. job_id=0 or an
// absent status filter means "do not filter on that field".
func (a *JobAPI) GetJobLogList(c *gin.Context) {
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

	var jobID uint
	if raw := c.Query("job_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			response.BadRequest(c, "job_id must be a positive integer")
			return
		}
		jobID = uint(parsed)
	}

	var success *int8
	if raw := c.Query("status"); raw != "" {
		st, err := strconv.Atoi(raw)
		if err != nil || (st != 0 && st != 1) {
			response.BadRequest(c, "status must be 0 or 1")
			return
		}
		st8 := int8(st)
		success = &st8
	}

	logs, total, err := a.service.GetJobLogListContext(c.Request.Context(), req, jobID, success)
	if err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, gin.H{
		"list":      logs,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

func (a *JobAPI) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, monitor.ErrInvalidCronExpression):
		response.BadRequest(c, monitor.ErrInvalidCronExpression.Error())
	case errors.Is(err, monitor.ErrInvalidRetentionDays):
		response.BadRequest(c, monitor.ErrInvalidRetentionDays.Error())
	case errors.Is(err, monitor.ErrUnknownInvokeTarget):
		// Caller input problem, not a server fault. The rejected value is
		// deliberately not echoed back: forwarding raw error text is what
		// TestAPIResponsesDoNotForwardRawErrors guards against.
		response.BadRequest(c, monitor.ErrUnknownInvokeTarget.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "job not found")
	default:
		internalServerError(c, "failed to process scheduled job request", err)
	}
}

// GetJobHeartbeats returns distributed job heartbeats: last-run status of
// in-process loops across services and host shell crons; stale means the job
// missed its expected schedule.
func (a *JobAPI) GetJobHeartbeats(c *gin.Context) {
	list, err := a.service.ListJobHeartbeatsContext(c.Request.Context())
	if err != nil {
		a.handleError(c, err)
		return
	}
	response.Success(c, gin.H{"list": list, "total": len(list)})
}
