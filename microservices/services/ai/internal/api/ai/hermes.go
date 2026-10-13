package ai

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/dao"
	"github.com/go-admin-kit/services/ai/internal/middleware"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

type HermesHandler struct {
	service *service.HermesService
}

func NewHermesHandler(hermesService *service.HermesService) *HermesHandler {
	return &HermesHandler{service: hermesService}
}

func (h *HermesHandler) CreateDeviceAuthorization(c *gin.Context) {
	var request service.CreateHermesDeviceAuthorizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid Hermes device authorization request")
		return
	}
	result, err := h.service.CreateDeviceAuthorization(c.Request.Context(), request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *HermesHandler) ExchangeDeviceCode(c *gin.Context) {
	var request struct {
		DeviceCode string `json:"device_code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid Hermes device token request")
		return
	}
	result, err := h.service.ExchangeDeviceCode(c.Request.Context(), request.DeviceCode)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *HermesHandler) ApproveDeviceAuthorization(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request struct {
		UserCode string `json:"user_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid Hermes device approval request")
		return
	}
	if err := h.service.ApproveDeviceAuthorization(
		c.Request.Context(),
		tenant.FromContextOrDefault(c.Request.Context()),
		userID,
		request.UserCode,
	); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 设备已获准绑定", nil)
}

func (h *HermesHandler) GetMe(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	result, err := h.service.GetMe(c.Request.Context(), userID)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *HermesHandler) ListMyDevices(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	result, err := h.service.GetMe(c.Request.Context(), userID)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, result.Devices)
}

func (h *HermesHandler) ListDownloads(c *gin.Context) {
	artifacts, _, err := h.service.ListArtifacts(c.Request.Context(), 1, 100, true)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, artifacts)
}

func (h *HermesHandler) RevokeMyDevice(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	if err := h.service.RevokeDevice(c.Request.Context(), c.Param("device_id"), &userID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 设备解绑请求已下发，等待设备确认", nil)
}

func (h *HermesHandler) ListEntitlements(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListEntitlements(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) UpdateEntitlement(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_USER"), "invalid user id")
		return
	}
	var request service.UpdateHermesEntitlementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid Hermes entitlement request")
		return
	}
	if err := h.service.UpdateEntitlement(c.Request.Context(), actorID, uint(userID), request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 权益已更新", nil)
}

func (h *HermesHandler) ListDevices(c *gin.Context) {
	page, pageSize := hermesPage(c)
	var userID *uint
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_USER"), "invalid user id")
			return
		}
		value := uint(parsed)
		userID = &value
	}
	items, total, err := h.service.ListDevices(c.Request.Context(), page, pageSize, userID)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) ListInstances(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListInstances(c.Request.Context(), page, pageSize, c.Query("device_id"))
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) CreateInstance(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.HermesInstanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	instance, err := h.service.CreateInstance(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, instance)
}

func (h *HermesHandler) UpdateInstance(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid instance id")
		return
	}
	var request service.HermesInstanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.UpdateInstance(c.Request.Context(), actorID, id, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "instance updated", nil)
}

func (h *HermesHandler) DeleteInstance(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid instance id")
		return
	}
	if err := h.service.DeleteInstance(c.Request.Context(), id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "instance deleted", nil)
}

func (h *HermesHandler) GetDevice(c *gin.Context) {
	device, events, err := h.service.GetDevice(c.Request.Context(), c.Param("device_id"))
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, gin.H{"device": device, "events": events})
}

func (h *HermesHandler) RevokeDevice(c *gin.Context) {
	if err := h.service.RevokeDevice(c.Request.Context(), c.Param("device_id"), nil); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 设备吊销请求已下发，等待设备确认", nil)
}

func (h *HermesHandler) ListPolicies(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListPolicies(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) CreatePolicy(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.CreateHermesPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid Hermes policy request")
		return
	}
	policy, err := h.service.CreatePolicy(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, policy)
}

// UpdatePolicy handles PUT /api/v1/ai/hermes/policies/:id.
func (h *HermesHandler) UpdatePolicy(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.CreateHermesPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	updated, err := h.service.UpdatePolicy(c.Request.Context(), userID, policyID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, updated)
}

// DeletePolicy handles DELETE /api/v1/ai/hermes/policies/:id.
func (h *HermesHandler) DeletePolicy(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	if err := h.service.DeletePolicy(c.Request.Context(), userID, policyID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "policy deleted", nil)
}

func (h *HermesHandler) ListArtifacts(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListArtifacts(c.Request.Context(), page, pageSize, false)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) ApproveArtifact(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	artifactID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || artifactID == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_ARTIFACT"), "invalid artifact id")
		return
	}
	if err := h.service.ApproveArtifact(c.Request.Context(), artifactID, actorID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 制品已批准", nil)
}

// RequestDeviceAction 给设备排一个受枚举约束的运维动作。请求体只接受 action
// 字段，不接受任何命令、参数或路径——控制面不具备下发任意执行内容的能力。
func (h *HermesHandler) RequestDeviceAction(c *gin.Context) {
	if _, ok := requireUserID(c); !ok {
		return
	}
	deviceID := strings.TrimSpace(c.Param("device_id"))
	var request struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid action payload")
		return
	}
	var err error
	if strings.TrimSpace(request.Action) == "" {
		err = h.service.CancelDeviceAction(c.Request.Context(), deviceID)
	} else {
		err = h.service.RequestDeviceAction(c.Request.Context(), deviceID, request.Action)
	}
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "运维动作已排入，设备下次同步时执行", nil)
}

func (h *HermesHandler) CreateArtifact(c *gin.Context) {
	if _, ok := requireUserID(c); !ok {
		return
	}
	var request service.CreateHermesArtifactRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_ARTIFACT"), "invalid artifact payload")
		return
	}
	artifact, err := h.service.CreateArtifact(c.Request.Context(), request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 制品已录入，等待审批", artifact)
}

func (h *HermesHandler) RejectArtifact(c *gin.Context) {
	h.transitionArtifact(c, "Hermes 制品已拒绝", func(ctx context.Context, id uint64, actor uint) error {
		return h.service.RejectArtifact(ctx, id, actor)
	})
}

func (h *HermesHandler) RevokeArtifact(c *gin.Context) {
	h.transitionArtifact(c, "Hermes 制品已吊销", func(ctx context.Context, id uint64, actor uint) error {
		return h.service.RevokeArtifact(ctx, id, actor)
	})
}

func (h *HermesHandler) transitionArtifact(c *gin.Context, okMessage string, fn func(context.Context, uint64, uint) error) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	artifactID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || artifactID == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_ARTIFACT"), "invalid artifact id")
		return
	}
	if err := fn(c.Request.Context(), artifactID, actorID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, okMessage, nil)
}

func (h *HermesHandler) DesiredState(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_CONTEXT_MISSING"), "Hermes device identity is missing")
		return
	}
	state, err := h.service.DesiredState(c.Request.Context(), device)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, state)
}

func (h *HermesHandler) Heartbeat(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_CONTEXT_MISSING"), "Hermes device identity is missing")
		return
	}
	var request service.HermesHeartbeatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_HEARTBEAT"), "invalid Hermes heartbeat")
		return
	}
	if err := h.service.RecordHeartbeat(c.Request.Context(), device.DeviceID, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, gin.H{"accepted": true})
}

func (h *HermesHandler) RecordEvent(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_CONTEXT_MISSING"), "Hermes device identity is missing")
		return
	}
	var request service.HermesEventRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_EVENT"), "invalid Hermes device event")
		return
	}
	if err := h.service.RecordEvent(c.Request.Context(), device.DeviceID, request); err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, gin.H{"accepted": true})
}

func hermesDeviceFromContext(c *gin.Context) (*localmodel.HermesDevice, bool) {
	value, exists := c.Get(middleware.HermesDeviceContextKey)
	if !exists {
		return nil, false
	}
	device, ok := value.(*localmodel.HermesDevice)
	return device, ok && device != nil
}

func hermesPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func writeHermesError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrHermesInvalidInput):
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), err.Error())
	case errors.Is(err, dao.ErrHermesAuthorizationPending):
		response.ErrorWithCode(c, http.StatusBadRequest, response.ErrorCode("HERMES_AUTHORIZATION_PENDING"), "device authorization is pending")
	case errors.Is(err, dao.ErrHermesAuthorizationExpired):
		response.ErrorWithCode(c, http.StatusBadRequest, response.ErrorCode("HERMES_AUTHORIZATION_EXPIRED"), "device authorization has expired")
	case errors.Is(err, dao.ErrHermesAuthorizationDenied):
		response.ForbiddenWithCode(c, response.ErrorCode("HERMES_AUTHORIZATION_DENIED"), "device authorization was denied or already consumed")
	case errors.Is(err, dao.ErrHermesEntitlementDisabled):
		response.ForbiddenWithCode(c, response.ErrorCode("HERMES_ENTITLEMENT_DISABLED"), "Hermes entitlement is disabled")
	case errors.Is(err, service.ErrHermesInstanceLimit):
		// 409 而非 400：请求本身合法，是该节点当前状态放不下第二个实例
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("HERMES_INSTANCE_LIMIT"),
			"该节点已有启用实例；gak-hermes-manager 暂不支持一节点多实例")
	case errors.Is(err, dao.ErrHermesDeviceLimitReached):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("HERMES_DEVICE_LIMIT_REACHED"), "Hermes device limit has been reached")
	case errors.Is(err, dao.ErrHermesDeviceRevoked):
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_REVOKED"), "Hermes device is revoked")
	case errors.Is(err, dao.ErrHermesArtifactDuplicate):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("HERMES_ARTIFACT_DUPLICATE"),
			"同类型/平台/架构下该版本已录入")
	case errors.Is(err, dao.ErrHermesSkillDuplicate):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("HERMES_SKILL_DUPLICATE"),
			"同技能名下该版本已录入")
	case errors.Is(err, dao.ErrHermesMcpDuplicate), errors.Is(err, dao.ErrHermesClusterMcpDuplicate):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("HERMES_MCP_DUPLICATE"),
			"同名 MCP 服务器已存在或已绑定")
	case errors.Is(err, service.ErrHermesControlPlaneUnavailable):
		response.ErrorWithCode(c, http.StatusServiceUnavailable, response.ErrorCode("HERMES_CONTROL_PLANE_UNAVAILABLE"), "Hermes policy signing is not configured")
	case errors.Is(err, service.ErrHermesPolicyNotConfigured):
		response.ErrorWithCode(c, http.StatusServiceUnavailable, response.ErrorCode("HERMES_POLICY_NOT_CONFIGURED"), "Hermes policy is not configured")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFoundWithCode(c, response.ErrorCode("HERMES_NOT_FOUND"), "Hermes resource was not found")
	default:
		response.InternalServerErrorWithCode(c, response.ErrorCode("HERMES_INTERNAL_ERROR"), err.Error())
	}
}
