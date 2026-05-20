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

func NewJobAPI() *JobAPI {
	return &JobAPI{
		service: monitor.GetJobService(),
	}
}

// GetJobList 获取任务列表
func (a *JobAPI) GetJobList(c *gin.Context) {
	var req pagination.PageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 设置默认分页参数
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

	jobs, total, err := a.service.GetJobList(req, name, status)
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

// CreateJob 创建任务
func (a *JobAPI) CreateJob(c *gin.Context) {
	var job model.ScheduledJob
	if err := c.ShouldBindJSON(&job); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.service.CreateJob(&job); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, job)
}

// UpdateJob 更新任务
func (a *JobAPI) UpdateJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	var job model.ScheduledJob
	if err := c.ShouldBindJSON(&job); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	job.ID = id

	if err := a.service.UpdateJob(&job); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, job)
}

// DeleteJob 删除任务
func (a *JobAPI) DeleteJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.DeleteJob(id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// StartJob 启动任务
func (a *JobAPI) StartJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.StartJobByID(id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// StopJob 停止任务
func (a *JobAPI) StopJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.StopJobByID(id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// RunJob 立即执行任务
func (a *JobAPI) RunJob(c *gin.Context) {
	id, ok := parseJobID(c)
	if !ok {
		return
	}

	if err := a.service.RunJob(id); err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, nil)
}

// GetJobHealth 获取任务健康检查结果
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

	health, err := a.service.CheckJobHealth(windowHours)
	if err != nil {
		a.handleError(c, err)
		return
	}

	response.Success(c, health)
}

// CleanupJobLogs 按保留天数清理任务日志
func (a *JobAPI) CleanupJobLogs(c *gin.Context) {
	retentionDays := monitor.DefaultJobLogRetentionDays

	var req cleanupJobLogsRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(c, err.Error())
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

	result, err := a.service.CleanupJobLogs(retentionDays)
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
		response.InternalServerError(c, err.Error())
	}
}
