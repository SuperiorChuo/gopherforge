package ai

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Hermes M5：Cron 定时任务的 console + device handlers。
// console 路由在 /api/v1/ai/hermes 下，经 protected + 动作级权限（cron:list/write）；
// device 上报在签名设备组 /api/v1/ai/hermes/device 下。

func (h *HermesHandler) ListCronJobs(c *gin.Context) {
	page, pageSize := hermesPage(c)
	jobs, total, err := h.service.ListCronJobs(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, jobs, total, page, pageSize)
}

func (h *HermesHandler) CreateCronJob(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.HermesCronJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	job, err := h.service.CreateCronJob(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "定时任务已创建", job)
}

func (h *HermesHandler) UpdateCronJob(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request service.HermesCronJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	job, err := h.service.UpdateCronJob(c.Request.Context(), actorID, id, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "定时任务已更新", job)
}

func (h *HermesHandler) ToggleCronJob(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	job, err := h.service.ToggleCronJob(c.Request.Context(), actorID, id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "定时任务状态已切换", job)
}

func (h *HermesHandler) DeleteCronJob(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCronJob(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "定时任务已删除", nil)
}

func (h *HermesHandler) ListCronRuns(c *gin.Context) {
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	page, pageSize := hermesPage(c)
	runs, total, err := h.service.ListCronRuns(c.Request.Context(), id, page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, runs, total, page, pageSize)
}

// ReportCronRun 是设备端 cron 执行结果的回传（签名设备通道）。
func (h *HermesHandler) ReportCronRun(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_UNAUTHORIZED"), "Hermes device authentication is required")
		return
	}
	var request struct {
		JobID  uint64 `json:"job_id"`
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.ReportDeviceCronRun(c.Request.Context(), device.DeviceID, request.JobID, request.Status, request.Result); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "定时任务执行已记录", nil)
}
