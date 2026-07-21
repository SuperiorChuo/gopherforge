package system

import (
	"context"

	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/pagination"
	"github.com/go-admin-kit/services/system/internal/pkg/tenant"
	"gorm.io/gorm"
)

// ---------- 短信渠道 ----------

type SmsChannelDAO struct {
	db *gorm.DB
}

func NewSmsChannelDAO(db *gorm.DB) *SmsChannelDAO {
	return &SmsChannelDAO{db: db}
}

func (d *SmsChannelDAO) GetByIDContext(ctx context.Context, id uint) (*model.SmsChannel, error) {
	var channel model.SmsChannel
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		First(&channel, id)
	return &channel, result.Error
}

func (d *SmsChannelDAO) GetListContext(ctx context.Context, req pagination.PageRequest, status *int8, provider, keyword string) ([]model.SmsChannel, int64, error) {
	var channels []model.SmsChannel
	var total int64

	query := d.dbWithContext(ctx).Model(&model.SmsChannel{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("id DESC").
		Find(&channels)

	return channels, total, result.Error
}

// GetEnabledListContext 返回启用中的渠道（模板表单下拉用）。
func (d *SmsChannelDAO) GetEnabledListContext(ctx context.Context) ([]model.SmsChannel, error) {
	var channels []model.SmsChannel
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Where("status = 1").
		Order("id ASC").
		Find(&channels)
	return channels, result.Error
}

func (d *SmsChannelDAO) CreateContext(ctx context.Context, channel *model.SmsChannel) error {
	if channel.TenantID == 0 {
		channel.TenantID = tenant.FromContextOrDefault(ctx)
	}
	return d.dbWithContext(ctx).Create(channel).Error
}

func (d *SmsChannelDAO) UpdateContext(ctx context.Context, channel *model.SmsChannel) error {
	return d.dbWithContext(ctx).Save(channel).Error
}

func (d *SmsChannelDAO) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&model.SmsChannel{}).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Update("status", status).Error
}

func (d *SmsChannelDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Delete(&model.SmsChannel{}, id).Error
}

func (d *SmsChannelDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// ---------- 短信模板 ----------

type SmsTemplateDAO struct {
	db *gorm.DB
}

func NewSmsTemplateDAO(db *gorm.DB) *SmsTemplateDAO {
	return &SmsTemplateDAO{db: db}
}

func (d *SmsTemplateDAO) GetByIDContext(ctx context.Context, id uint) (*model.SmsTemplate, error) {
	var template model.SmsTemplate
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		First(&template, id)
	return &template, result.Error
}

func (d *SmsTemplateDAO) GetByCodeContext(ctx context.Context, code string) (*model.SmsTemplate, error) {
	var template model.SmsTemplate
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Where("code = ?", code).
		First(&template)
	return &template, result.Error
}

func (d *SmsTemplateDAO) GetListContext(ctx context.Context, req pagination.PageRequest, channelID *uint, templateType *int8, status *int8, keyword string) ([]model.SmsTemplate, int64, error) {
	var templates []model.SmsTemplate
	var total int64

	query := d.dbWithContext(ctx).Model(&model.SmsTemplate{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if channelID != nil {
		query = query.Where("channel_id = ?", *channelID)
	}
	if templateType != nil {
		query = query.Where("type = ?", *templateType)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if keyword != "" {
		query = query.Where("code LIKE ? OR name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("id DESC").
		Find(&templates)

	return templates, total, result.Error
}

// CountByCodeContext 统计同租户内使用某 code 的模板数（唯一性校验用，excludeID 排除自身）。
func (d *SmsTemplateDAO) CountByCodeContext(ctx context.Context, code string, excludeID uint) (int64, error) {
	var count int64
	query := d.dbWithContext(ctx).Model(&model.SmsTemplate{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Where("code = ?", code)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Count(&count).Error
	return count, err
}

// CountByChannelContext 统计引用某渠道的模板数（删渠道前的引用检查）。
func (d *SmsTemplateDAO) CountByChannelContext(ctx context.Context, channelID uint) (int64, error) {
	var count int64
	err := d.dbWithContext(ctx).Model(&model.SmsTemplate{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Where("channel_id = ?", channelID).
		Count(&count).Error
	return count, err
}

func (d *SmsTemplateDAO) CreateContext(ctx context.Context, template *model.SmsTemplate) error {
	if template.TenantID == 0 {
		template.TenantID = tenant.FromContextOrDefault(ctx)
	}
	return d.dbWithContext(ctx).Create(template).Error
}

func (d *SmsTemplateDAO) UpdateContext(ctx context.Context, template *model.SmsTemplate) error {
	return d.dbWithContext(ctx).Save(template).Error
}

func (d *SmsTemplateDAO) UpdateStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&model.SmsTemplate{}).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Update("status", status).Error
}

func (d *SmsTemplateDAO) DeleteContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		Delete(&model.SmsTemplate{}, id).Error
}

func (d *SmsTemplateDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// ---------- 发送日志 ----------

type SmsLogDAO struct {
	db *gorm.DB
}

func NewSmsLogDAO(db *gorm.DB) *SmsLogDAO {
	return &SmsLogDAO{db: db}
}

func (d *SmsLogDAO) CreateContext(ctx context.Context, log *model.SmsLog) error {
	if log.TenantID == 0 {
		log.TenantID = tenant.FromContextOrDefault(ctx)
	}
	return d.dbWithContext(ctx).Create(log).Error
}

// UpdateResultContext 回写发送结果（状态 + 厂商回执 / 错误信息）。
func (d *SmsLogDAO) UpdateResultContext(ctx context.Context, id uint, status, providerMsgID, errMsg string) error {
	return d.dbWithContext(ctx).Model(&model.SmsLog{}).
		Where("id = ? AND tenant_id = ?", id, tenant.FromContextOrDefault(ctx)).
		Updates(map[string]any{
			"status":          status,
			"provider_msg_id": providerMsgID,
			"error":           errMsg,
		}).Error
}

func (d *SmsLogDAO) GetByIDContext(ctx context.Context, id uint) (*model.SmsLog, error) {
	var log model.SmsLog
	result := d.dbWithContext(ctx).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx)).
		First(&log, id)
	return &log, result.Error
}

func (d *SmsLogDAO) GetListContext(ctx context.Context, req pagination.PageRequest, mobile, templateCode, status string) ([]model.SmsLog, int64, error) {
	var logs []model.SmsLog
	var total int64

	query := d.dbWithContext(ctx).Model(&model.SmsLog{}).
		Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
	if mobile != "" {
		query = query.Where("mobile LIKE ?", "%"+mobile+"%")
	}
	if templateCode != "" {
		query = query.Where("template_code = ?", templateCode)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Order("id DESC").
		Find(&logs)

	return logs, total, result.Error
}

func (d *SmsLogDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}
