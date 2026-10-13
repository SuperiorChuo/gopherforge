package ai

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Hermes 第二期：多 Agent 协作任务的 console handlers。所有路由在
// /api/v1/ai/hermes 下，经 protected（JWT + OperationLogger）与动作级权限。

// ---------------------------------------------------------------------------
// 集群
// ---------------------------------------------------------------------------

func (h *HermesHandler) ListClusters(c *gin.Context) {
	page, pageSize := hermesPage(c)
	clusters, total, err := h.service.ListClusters(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, clusters, total, page, pageSize)
}

func (h *HermesHandler) CreateCluster(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.HermesClusterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	cluster, err := h.service.CreateCluster(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 集群已创建", cluster)
}

func (h *HermesHandler) GetCluster(c *gin.Context) {
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	cluster, instances, err := h.service.GetCluster(c.Request.Context(), id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, gin.H{"cluster": cluster, "instances": instances})
}

func (h *HermesHandler) UpdateCluster(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request service.HermesClusterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.UpdateCluster(c.Request.Context(), actorID, id, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 集群已更新", nil)
}

func (h *HermesHandler) DeleteCluster(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCluster(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 集群已删除", nil)
}

// ---------------------------------------------------------------------------
// 任务
// ---------------------------------------------------------------------------

func (h *HermesHandler) ListTasks(c *gin.Context) {
	page, pageSize := hermesPage(c)
	tasks, total, err := h.service.ListTasks(c.Request.Context(), page, pageSize, c.Query("status"))
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, tasks, total, page, pageSize)
}

func (h *HermesHandler) CreateTask(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.HermesTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	task, err := h.service.CreateTask(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 任务已创建", task)
}

func (h *HermesHandler) GetTask(c *gin.Context) {
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	detail, err := h.service.GetTask(c.Request.Context(), id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *HermesHandler) ActivateTask(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	task, err := h.service.ActivateTask(c.Request.Context(), actorID, id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 任务已激活", task)
}

func (h *HermesHandler) CompleteTask(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.CompleteTask(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 任务已完成", nil)
}

func (h *HermesHandler) CancelTask(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.CancelTask(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 任务已取消", nil)
}

// ---------------------------------------------------------------------------
// 任务执行记录
// ---------------------------------------------------------------------------

func (h *HermesHandler) UpdateTaskRun(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request service.HermesTaskRunTransitionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.UpdateTaskRun(c.Request.Context(), actorID, id, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "任务执行状态已更新", nil)
}

// ReportTaskRun 是设备端执行完任务后的结果回传。走设备鉴权组（非 console），
// 设备身份由 hermesDeviceFromContext 提供，不接受客户端自报的 device_id。
func (h *HermesHandler) ReportTaskRun(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_UNAUTHORIZED"), "Hermes device authentication is required")
		return
	}
	var request struct {
		RunID  uint64 `json:"run_id"`
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.ReportDeviceTaskRun(c.Request.Context(), device.DeviceID, request.RunID, request.Status, request.Result); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "任务结果已记录", nil)
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

func parseHermesID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid id")
		return 0, false
	}
	return id, true
}
