package ai

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Hermes M3 MCP 管理的 console + device handlers。

func (h *HermesHandler) ListMcpServers(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListMcpServers(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) CreateMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.CreateHermesMcpServerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	server, err := h.service.CreateMcpServer(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes MCP 服务器已录入，等待审批", server)
}

func (h *HermesHandler) UpdateMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request service.CreateHermesMcpServerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.UpdateMcpServer(c.Request.Context(), actorID, id, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes MCP 服务器已更新，等待重审批", nil)
}

func (h *HermesHandler) ApproveMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.ApproveMcpServer(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes MCP 服务器已批准", nil)
}

func (h *HermesHandler) RejectMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.RejectMcpServer(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes MCP 服务器已拒绝", nil)
}

func (h *HermesHandler) RevokeMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.RevokeMcpServer(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes MCP 服务器已吊销", nil)
}

func (h *HermesHandler) ListClusterMcpServers(c *gin.Context) {
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	items, err := h.service.ListClusterMcpServers(c.Request.Context(), id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *HermesHandler) BindClusterMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	clusterID, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request struct {
		ServerID uint64 `json:"server_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.BindClusterMcpServer(c.Request.Context(), actorID, clusterID, request.ServerID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "MCP 服务器已绑定到集群", nil)
}

func (h *HermesHandler) UnbindClusterMcpServer(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	clusterID, ok := parseHermesID(c)
	if !ok {
		return
	}
	serverID, err := strconv.ParseUint(c.Param("server_id"), 10, 64)
	if err != nil || serverID == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid server id")
		return
	}
	if err := h.service.UnbindClusterMcpServer(c.Request.Context(), actorID, clusterID, serverID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "MCP 服务器已从集群解绑", nil)
}

// IssueMcpSecrets 是设备签名端点：manager 按 desired-state 里的 ${VAR} 键拉取
// 密钥值，写进 profile .env。只返回请求键与集群授权键的交集。
func (h *HermesHandler) IssueMcpSecrets(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_UNAUTHORIZED"), "Hermes device authentication is required")
		return
	}
	var request struct {
		EnvKeys []string `json:"env_keys"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	secrets, err := h.service.IssueMcpSecrets(c.Request.Context(), device.DeviceID, request.EnvKeys)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, gin.H{"secrets": secrets})
}
