package system

import (
	"context"
	"errors"
	"fmt"
	"strings"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
	"github.com/go-admin-kit/services/system/internal/pkg/sms"
	"gorm.io/gorm"
)

// 短信模块业务错误。
var (
	ErrSmsChannelNotFound    = errors.New("sms channel not found")
	ErrSmsChannelDisabled    = errors.New("sms channel is disabled")
	ErrSmsChannelInUse       = errors.New("sms channel is referenced by templates")
	ErrSmsProviderInvalid    = errors.New("invalid sms provider")
	ErrSmsTemplateNotFound   = errors.New("sms template not found")
	ErrSmsTemplateCodeExists = errors.New("sms template code already exists")
	ErrSmsTemplateDisabled   = errors.New("sms template is disabled")
	ErrSmsParamsMissing      = errors.New("sms template params missing")
)

// smsSecretMask 是密钥回显占位：读取时替换真实密钥；更新时收到它则保留旧值。
const smsSecretMask = "******"

// smsSensitiveConfigKeys 是渠道 config 里需要脱敏的 key。
var smsSensitiveConfigKeys = map[string]struct{}{
	"access_key_secret": {}, // aliyun
	"secret_key":        {}, // tencent
}

// validSmsProviders 与 pkg/sms 支持的 provider 一致。
var validSmsProviders = map[string]struct{}{
	sms.ProviderDebug:   {},
	sms.ProviderAliyun:  {},
	sms.ProviderTencent: {},
}

// maskSmsChannelConfig 返回脱敏后的 config 副本（不改原 map）。
func maskSmsChannelConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}
	masked := make(map[string]any, len(config))
	for k, v := range config {
		if _, sensitive := smsSensitiveConfigKeys[k]; sensitive {
			if s, ok := v.(string); ok && s != "" {
				masked[k] = smsSecretMask
				continue
			}
		}
		masked[k] = v
	}
	return masked
}

// mergeSmsChannelSecrets 把更新请求里被脱敏（或留空）的密钥项还原为库里的旧值。
func mergeSmsChannelSecrets(incoming, existing map[string]any) map[string]any {
	if incoming == nil {
		return existing
	}
	merged := make(map[string]any, len(incoming))
	for k, v := range incoming {
		merged[k] = v
	}
	for k := range smsSensitiveConfigKeys {
		s, ok := merged[k].(string)
		if ok && s != "" && s != smsSecretMask {
			continue // 用户提供了新密钥
		}
		if old, exists := existing[k]; exists {
			merged[k] = old
		}
	}
	return merged
}

// maskSmsChannel 返回脱敏后的渠道副本。
func maskSmsChannel(channel model.SmsChannel) model.SmsChannel {
	channel.Config = maskSmsChannelConfig(channel.Config)
	return channel
}

// ---------- 渠道 ----------

type SmsChannelService struct {
	channelDAO  systemdao.SmsChannelDAO
	templateDAO systemdao.SmsTemplateDAO
}

// NewSmsChannelServiceWithDB builds a SmsChannelService backed by an injected database handle.
func NewSmsChannelServiceWithDB(db *gorm.DB) SmsChannelService {
	return SmsChannelService{
		channelDAO:  *systemdao.NewSmsChannelDAO(db),
		templateDAO: *systemdao.NewSmsTemplateDAO(db),
	}
}

type SmsChannelListRequest struct {
	pagination.PageRequest
	Status   *int8  `json:"status" form:"status"`
	Provider string `json:"provider" form:"provider"`
	Keyword  string `json:"keyword" form:"keyword"`
}

type CreateSmsChannelRequest struct {
	Name     string         `json:"name" binding:"required"`
	Provider string         `json:"provider" binding:"required"`
	Config   map[string]any `json:"config"`
	Status   int8           `json:"status"`
	Remark   string         `json:"remark"`
}

type UpdateSmsChannelRequest struct {
	Name     string         `json:"name"`
	Provider string         `json:"provider"`
	Config   map[string]any `json:"config"`
	Status   int8           `json:"status"`
	Remark   string         `json:"remark"`
}

// GetListContext 返回分页渠道列表（config 已脱敏）。
func (s *SmsChannelService) GetListContext(ctx context.Context, req SmsChannelListRequest) ([]model.SmsChannel, int64, error) {
	channels, total, err := s.channelDAO.GetListContext(ctx, req.PageRequest, req.Status, req.Provider, req.Keyword)
	if err != nil {
		return nil, 0, err
	}
	for i := range channels {
		channels[i] = maskSmsChannel(channels[i])
	}
	return channels, total, nil
}

// GetEnabledListContext 返回启用渠道（模板表单下拉用，config 已脱敏）。
func (s *SmsChannelService) GetEnabledListContext(ctx context.Context) ([]model.SmsChannel, error) {
	channels, err := s.channelDAO.GetEnabledListContext(ctx)
	if err != nil {
		return nil, err
	}
	for i := range channels {
		channels[i] = maskSmsChannel(channels[i])
	}
	return channels, nil
}

func (s *SmsChannelService) GetByIDContext(ctx context.Context, id uint) (*model.SmsChannel, error) {
	channel, err := s.channelDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsChannelNotFound
		}
		return nil, err
	}
	masked := maskSmsChannel(*channel)
	return &masked, nil
}

func (s *SmsChannelService) CreateContext(ctx context.Context, req CreateSmsChannelRequest) (*model.SmsChannel, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if _, ok := validSmsProviders[provider]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrSmsProviderInvalid, req.Provider)
	}
	if req.Status == 0 {
		req.Status = 1
	}

	channel := &model.SmsChannel{
		Name:     req.Name,
		Provider: provider,
		Config:   req.Config,
		Status:   req.Status,
		Remark:   req.Remark,
	}
	if err := s.channelDAO.CreateContext(ctx, channel); err != nil {
		return nil, err
	}
	masked := maskSmsChannel(*channel)
	return &masked, nil
}

func (s *SmsChannelService) UpdateContext(ctx context.Context, id uint, req UpdateSmsChannelRequest) (*model.SmsChannel, error) {
	channel, err := s.channelDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsChannelNotFound
		}
		return nil, err
	}

	if req.Name != "" {
		channel.Name = req.Name
	}
	if req.Provider != "" {
		provider := strings.ToLower(strings.TrimSpace(req.Provider))
		if _, ok := validSmsProviders[provider]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrSmsProviderInvalid, req.Provider)
		}
		channel.Provider = provider
	}
	if req.Config != nil {
		channel.Config = mergeSmsChannelSecrets(req.Config, channel.Config)
	}
	channel.Status = req.Status
	channel.Remark = req.Remark

	if err := s.channelDAO.UpdateContext(ctx, channel); err != nil {
		return nil, err
	}
	masked := maskSmsChannel(*channel)
	return &masked, nil
}

func (s *SmsChannelService) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return s.channelDAO.UpdateStatusContext(ctx, id, status)
}

// DeleteContext 删除渠道；仍被模板引用时拒绝。
func (s *SmsChannelService) DeleteContext(ctx context.Context, id uint) error {
	count, err := s.templateDAO.CountByChannelContext(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrSmsChannelInUse
	}
	return s.channelDAO.DeleteContext(ctx, id)
}

// ---------- 模板 ----------

type SmsTemplateService struct {
	templateDAO systemdao.SmsTemplateDAO
	channelDAO  systemdao.SmsChannelDAO
}

// NewSmsTemplateServiceWithDB builds a SmsTemplateService backed by an injected database handle.
func NewSmsTemplateServiceWithDB(db *gorm.DB) SmsTemplateService {
	return SmsTemplateService{
		templateDAO: *systemdao.NewSmsTemplateDAO(db),
		channelDAO:  *systemdao.NewSmsChannelDAO(db),
	}
}

type SmsTemplateListRequest struct {
	pagination.PageRequest
	ChannelID *uint  `json:"channel_id" form:"channel_id"`
	Type      *int8  `json:"type" form:"type"`
	Status    *int8  `json:"status" form:"status"`
	Keyword   string `json:"keyword" form:"keyword"`
}

type CreateSmsTemplateRequest struct {
	Code               string `json:"code" binding:"required"`
	Name               string `json:"name" binding:"required"`
	ChannelID          uint   `json:"channel_id" binding:"required"`
	Content            string `json:"content" binding:"required"`
	Type               int8   `json:"type"`
	ProviderTemplateID string `json:"provider_template_id"`
	Status             int8   `json:"status"`
	Remark             string `json:"remark"`
}

type UpdateSmsTemplateRequest struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	ChannelID          uint   `json:"channel_id"`
	Content            string `json:"content"`
	Type               int8   `json:"type"`
	ProviderTemplateID string `json:"provider_template_id"`
	Status             int8   `json:"status"`
	Remark             string `json:"remark"`
}

func (s *SmsTemplateService) GetListContext(ctx context.Context, req SmsTemplateListRequest) ([]model.SmsTemplate, int64, error) {
	return s.templateDAO.GetListContext(ctx, req.PageRequest, req.ChannelID, req.Type, req.Status, req.Keyword)
}

func (s *SmsTemplateService) GetByIDContext(ctx context.Context, id uint) (*model.SmsTemplate, error) {
	template, err := s.templateDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsTemplateNotFound
		}
		return nil, err
	}
	return template, nil
}

func (s *SmsTemplateService) CreateContext(ctx context.Context, req CreateSmsTemplateRequest) (*model.SmsTemplate, error) {
	count, err := s.templateDAO.CountByCodeContext(ctx, req.Code, 0)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSmsTemplateCodeExists
	}
	if _, err := s.channelDAO.GetByIDContext(ctx, req.ChannelID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsChannelNotFound
		}
		return nil, err
	}
	if req.Type == 0 {
		req.Type = model.SmsTemplateTypeNotify
	}
	if req.Status == 0 {
		req.Status = 1
	}

	template := &model.SmsTemplate{
		Code:               req.Code,
		Name:               req.Name,
		ChannelID:          req.ChannelID,
		Content:            req.Content,
		Type:               req.Type,
		ProviderTemplateID: req.ProviderTemplateID,
		Status:             req.Status,
		Remark:             req.Remark,
	}
	if err := s.templateDAO.CreateContext(ctx, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *SmsTemplateService) UpdateContext(ctx context.Context, id uint, req UpdateSmsTemplateRequest) (*model.SmsTemplate, error) {
	template, err := s.templateDAO.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsTemplateNotFound
		}
		return nil, err
	}

	if req.Code != "" && req.Code != template.Code {
		count, err := s.templateDAO.CountByCodeContext(ctx, req.Code, id)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrSmsTemplateCodeExists
		}
		template.Code = req.Code
	}
	if req.ChannelID != 0 && req.ChannelID != template.ChannelID {
		if _, err := s.channelDAO.GetByIDContext(ctx, req.ChannelID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrSmsChannelNotFound
			}
			return nil, err
		}
		template.ChannelID = req.ChannelID
	}
	if req.Name != "" {
		template.Name = req.Name
	}
	if req.Content != "" {
		template.Content = req.Content
	}
	if req.Type != 0 {
		template.Type = req.Type
	}
	template.ProviderTemplateID = req.ProviderTemplateID
	template.Status = req.Status
	template.Remark = req.Remark

	if err := s.templateDAO.UpdateContext(ctx, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *SmsTemplateService) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return s.templateDAO.UpdateStatusContext(ctx, id, status)
}

func (s *SmsTemplateService) DeleteContext(ctx context.Context, id uint) error {
	return s.templateDAO.DeleteContext(ctx, id)
}

// ---------- 发送日志 ----------

type SmsLogService struct {
	logDAO systemdao.SmsLogDAO
}

// NewSmsLogServiceWithDB builds a SmsLogService backed by an injected database handle.
func NewSmsLogServiceWithDB(db *gorm.DB) SmsLogService {
	return SmsLogService{logDAO: *systemdao.NewSmsLogDAO(db)}
}

type SmsLogListRequest struct {
	pagination.PageRequest
	Mobile       string `json:"mobile" form:"mobile"`
	TemplateCode string `json:"template_code" form:"template_code"`
	Status       string `json:"status" form:"status"`
}

func (s *SmsLogService) GetListContext(ctx context.Context, req SmsLogListRequest) ([]model.SmsLog, int64, error) {
	return s.logDAO.GetListContext(ctx, req.PageRequest, req.Mobile, req.TemplateCode, req.Status)
}

// ---------- 发送编排 ----------

// SmsSendService 是发送入口：渲染模板 → 选渠道 → 发送 → 写日志。
type SmsSendService struct {
	channelDAO  systemdao.SmsChannelDAO
	templateDAO systemdao.SmsTemplateDAO
	logDAO      systemdao.SmsLogDAO
	// newSender 可注入（单测替换为假发送器）。
	newSender func(provider string, config map[string]any) (sms.Sender, error)
}

// NewSmsSendServiceWithDB builds a SmsSendService backed by an injected database handle.
func NewSmsSendServiceWithDB(db *gorm.DB) SmsSendService {
	return SmsSendService{
		channelDAO:  *systemdao.NewSmsChannelDAO(db),
		templateDAO: *systemdao.NewSmsTemplateDAO(db),
		logDAO:      *systemdao.NewSmsLogDAO(db),
		newSender:   sms.NewSenderFromConfig,
	}
}

type SendSmsRequest struct {
	Mobile       string            `json:"mobile" binding:"required"`
	TemplateCode string            `json:"template_code" binding:"required"`
	Params       map[string]string `json:"params"`
}

// SendSmsResult 是发送结果：无论成败都有对应日志；发送失败属业务结果，
// 不作为 API 错误返回（前置校验失败除外）。
type SendSmsResult struct {
	LogID         uint   `json:"log_id"`
	Status        string `json:"status"`
	Content       string `json:"content"`
	ProviderMsgID string `json:"provider_msg_id,omitempty"`
	Error         string `json:"error,omitempty"`
}

// SendContext 发送一条短信。前置校验（模板/渠道/参数）失败返回 error 且不写日志；
// 进入发送阶段后（含构造发送器失败），结果一律落日志并通过 SendSmsResult 返回。
func (s *SmsSendService) SendContext(ctx context.Context, req SendSmsRequest) (*SendSmsResult, error) {
	template, err := s.templateDAO.GetByCodeContext(ctx, req.TemplateCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsTemplateNotFound
		}
		return nil, err
	}
	if template.Status != 1 {
		return nil, ErrSmsTemplateDisabled
	}

	channel, err := s.channelDAO.GetByIDContext(ctx, template.ChannelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSmsChannelNotFound
		}
		return nil, err
	}
	if channel.Status != 1 {
		return nil, ErrSmsChannelDisabled
	}

	if missing := sms.MissingParams(template.Content, req.Params); len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrSmsParamsMissing, strings.Join(missing, ", "))
	}
	content := sms.RenderTemplate(template.Content, req.Params)

	// 先落一条 sending 日志，保证每次发送尝试可追溯。
	log := &model.SmsLog{
		Mobile:       req.Mobile,
		TemplateCode: template.Code,
		Content:      content,
		Params:       req.Params,
		ChannelID:    channel.ID,
		ChannelName:  channel.Name,
		Provider:     channel.Provider,
		Status:       model.SmsStatusSending,
	}
	if err := s.logDAO.CreateContext(ctx, log); err != nil {
		return nil, err
	}

	sender, err := s.newSender(channel.Provider, channel.Config)
	if err != nil {
		return s.finishSend(ctx, log, content, "", err)
	}
	result, sendErr := sender.Send(ctx, sms.SendRequest{
		Mobile:             req.Mobile,
		Params:             req.Params,
		Content:            content,
		ProviderTemplateID: template.ProviderTemplateID,
	})
	msgID := ""
	if result != nil {
		msgID = result.MessageID
	}
	return s.finishSend(ctx, log, content, msgID, sendErr)
}

// finishSend 回写日志并组装返回值。
func (s *SmsSendService) finishSend(ctx context.Context, log *model.SmsLog, content, msgID string, sendErr error) (*SendSmsResult, error) {
	status := model.SmsStatusSuccess
	errMsg := ""
	if sendErr != nil {
		status = model.SmsStatusFailure
		errMsg = truncateSmsError(sendErr.Error())
	}
	// 结果回写失败只能如实报错：日志状态比响应体更重要（审计口径）。
	if err := s.logDAO.UpdateResultContext(ctx, log.ID, status, msgID, errMsg); err != nil {
		return nil, err
	}
	return &SendSmsResult{
		LogID:         log.ID,
		Status:        status,
		Content:       content,
		ProviderMsgID: msgID,
		Error:         errMsg,
	}, nil
}

// truncateSmsError 截断超长错误串，适配 sms_logs.error 列宽（512）。
func truncateSmsError(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
