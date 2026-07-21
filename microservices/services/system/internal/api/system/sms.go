package system

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"gorm.io/gorm"
)

// SmsAPI 短信管理端点：渠道 / 模板 / 发送日志 / 发送入口。
type SmsAPI struct {
	channelService  systemsvc.SmsChannelService
	templateService systemsvc.SmsTemplateService
	logService      systemsvc.SmsLogService
	sendService     systemsvc.SmsSendService
}

// NewSmsAPI creates a SmsAPI instance with zero-value services (legacy fallback).
func NewSmsAPI() *SmsAPI {
	return &SmsAPI{}
}

// NewSmsAPIWithDB creates a SmsAPI instance from an injected database handle.
func NewSmsAPIWithDB(db *gorm.DB) *SmsAPI {
	return &SmsAPI{
		channelService:  systemsvc.NewSmsChannelServiceWithDB(db),
		templateService: systemsvc.NewSmsTemplateServiceWithDB(db),
		logService:      systemsvc.NewSmsLogServiceWithDB(db),
		sendService:     systemsvc.NewSmsSendServiceWithDB(db),
	}
}

// writeSmsServiceError 把短信模块业务错误映射为 HTTP 响应（独立于共享
// error_response.go，避免并行开发冲突）。
func writeSmsServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrSmsChannelNotFound),
		errors.Is(err, systemsvc.ErrSmsTemplateNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, systemsvc.ErrSmsChannelInUse),
		errors.Is(err, systemsvc.ErrSmsChannelDisabled),
		errors.Is(err, systemsvc.ErrSmsProviderInvalid),
		errors.Is(err, systemsvc.ErrSmsTemplateCodeExists),
		errors.Is(err, systemsvc.ErrSmsTemplateDisabled),
		errors.Is(err, systemsvc.ErrSmsParamsMissing):
		response.BadRequest(c, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

// parseSmsID 解析路径里的 :id。
func parseSmsID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}

// parseSmsStatusBody 解析启停请求体。
func parseSmsStatusBody(c *gin.Context) (int8, bool) {
	var req struct {
		Status int8 `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return 0, false
	}
	return req.Status, true
}

// ---------- 渠道 ----------

// GetChannelList 返回分页渠道列表（config 已脱敏）。
func (a *SmsAPI) GetChannelList(c *gin.Context) {
	var req systemsvc.SmsChannelListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	if statusStr := c.Query("status"); statusStr != "" {
		if s, err := strconv.ParseInt(statusStr, 10, 8); err == nil {
			sInt8 := int8(s)
			req.Status = &sInt8
		}
	}

	channels, total, err := a.channelService.GetListContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to get sms channel list", err)
		return
	}

	response.Success(c, gin.H{
		"list":      channels,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetEnabledChannels 返回启用渠道（模板表单下拉用）。
func (a *SmsAPI) GetEnabledChannels(c *gin.Context) {
	channels, err := a.channelService.GetEnabledListContext(c.Request.Context())
	if err != nil {
		writeSmsServiceError(c, "failed to get enabled sms channels", err)
		return
	}
	response.Success(c, channels)
}

// GetChannel 返回渠道详情（config 已脱敏）。
func (a *SmsAPI) GetChannel(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	channel, err := a.channelService.GetByIDContext(c.Request.Context(), id)
	if err != nil {
		writeSmsServiceError(c, "failed to get sms channel", err)
		return
	}
	response.Success(c, channel)
}

// CreateChannel 创建渠道。
func (a *SmsAPI) CreateChannel(c *gin.Context) {
	var req systemsvc.CreateSmsChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	channel, err := a.channelService.CreateContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to create sms channel", err)
		return
	}
	response.SuccessWithMessage(c, "sms channel created", channel)
}

// UpdateChannel 更新渠道（密钥回传脱敏占位时保留旧值）。
func (a *SmsAPI) UpdateChannel(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	var req systemsvc.UpdateSmsChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	channel, err := a.channelService.UpdateContext(c.Request.Context(), id, req)
	if err != nil {
		writeSmsServiceError(c, "failed to update sms channel", err)
		return
	}
	response.SuccessWithMessage(c, "sms channel updated", channel)
}

// UpdateChannelStatus 启停渠道。
func (a *SmsAPI) UpdateChannelStatus(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	status, ok := parseSmsStatusBody(c)
	if !ok {
		return
	}
	if err := a.channelService.UpdateStatusContext(c.Request.Context(), id, status); err != nil {
		writeSmsServiceError(c, "failed to update sms channel status", err)
		return
	}
	response.SuccessWithMessage(c, "sms channel status updated", nil)
}

// DeleteChannel 删除渠道（被模板引用时 400）。
func (a *SmsAPI) DeleteChannel(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	if err := a.channelService.DeleteContext(c.Request.Context(), id); err != nil {
		writeSmsServiceError(c, "failed to delete sms channel", err)
		return
	}
	response.SuccessWithMessage(c, "sms channel deleted", nil)
}

// ---------- 模板 ----------

// GetTemplateList 返回分页模板列表。
func (a *SmsAPI) GetTemplateList(c *gin.Context) {
	var req systemsvc.SmsTemplateListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	if typeStr := c.Query("type"); typeStr != "" {
		if v, err := strconv.ParseInt(typeStr, 10, 8); err == nil {
			vInt8 := int8(v)
			req.Type = &vInt8
		}
	}
	if statusStr := c.Query("status"); statusStr != "" {
		if v, err := strconv.ParseInt(statusStr, 10, 8); err == nil {
			vInt8 := int8(v)
			req.Status = &vInt8
		}
	}
	if channelStr := c.Query("channel_id"); channelStr != "" {
		if v, err := strconv.ParseUint(channelStr, 10, 32); err == nil {
			vUint := uint(v)
			req.ChannelID = &vUint
		}
	}

	templates, total, err := a.templateService.GetListContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to get sms template list", err)
		return
	}

	response.Success(c, gin.H{
		"list":      templates,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetTemplate 返回模板详情。
func (a *SmsAPI) GetTemplate(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	template, err := a.templateService.GetByIDContext(c.Request.Context(), id)
	if err != nil {
		writeSmsServiceError(c, "failed to get sms template", err)
		return
	}
	response.Success(c, template)
}

// CreateTemplate 创建模板（code 租户内唯一）。
func (a *SmsAPI) CreateTemplate(c *gin.Context) {
	var req systemsvc.CreateSmsTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	template, err := a.templateService.CreateContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to create sms template", err)
		return
	}
	response.SuccessWithMessage(c, "sms template created", template)
}

// UpdateTemplate 更新模板。
func (a *SmsAPI) UpdateTemplate(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	var req systemsvc.UpdateSmsTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	template, err := a.templateService.UpdateContext(c.Request.Context(), id, req)
	if err != nil {
		writeSmsServiceError(c, "failed to update sms template", err)
		return
	}
	response.SuccessWithMessage(c, "sms template updated", template)
}

// UpdateTemplateStatus 启停模板。
func (a *SmsAPI) UpdateTemplateStatus(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	status, ok := parseSmsStatusBody(c)
	if !ok {
		return
	}
	if err := a.templateService.UpdateStatusContext(c.Request.Context(), id, status); err != nil {
		writeSmsServiceError(c, "failed to update sms template status", err)
		return
	}
	response.SuccessWithMessage(c, "sms template status updated", nil)
}

// DeleteTemplate 删除模板。
func (a *SmsAPI) DeleteTemplate(c *gin.Context) {
	id, ok := parseSmsID(c)
	if !ok {
		return
	}
	if err := a.templateService.DeleteContext(c.Request.Context(), id); err != nil {
		writeSmsServiceError(c, "failed to delete sms template", err)
		return
	}
	response.SuccessWithMessage(c, "sms template deleted", nil)
}

// ---------- 发送日志 ----------

// GetLogList 返回分页发送日志（按手机号/模板/状态筛选）。
func (a *SmsAPI) GetLogList(c *gin.Context) {
	var req systemsvc.SmsLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	logs, total, err := a.logService.GetListContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to get sms log list", err)
		return
	}

	response.Success(c, gin.H{
		"list":      logs,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// ---------- 发送 ----------

// SendSms 发送短信（同时用作模板测试发送）：渲染 → 选渠道 → 发送 → 写日志。
// 云厂商拒绝属业务结果，返回 200 + status=failure；前置校验失败返回 4xx。
func (a *SmsAPI) SendSms(c *gin.Context) {
	var req systemsvc.SendSmsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	result, err := a.sendService.SendContext(c.Request.Context(), req)
	if err != nil {
		writeSmsServiceError(c, "failed to send sms", err)
		return
	}
	response.Success(c, result)
}
