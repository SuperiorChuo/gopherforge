package model

import "time"

// 短信发送状态（sms_logs.status 用字符串，语义比数字直观，方便日志检索）。
const (
	SmsStatusSending = "sending" // 已提交、结果未知
	SmsStatusSuccess = "success" // 云厂商受理成功
	SmsStatusFailure = "failure" // 发送失败（渲染/网络/云厂商拒绝）
)

// 短信模板类型：与前端下拉一致（1 验证码 / 2 通知 / 3 营销）。
const (
	SmsTemplateTypeCode   int8 = 1
	SmsTemplateTypeNotify int8 = 2
	SmsTemplateTypeMarket int8 = 3
)

// SmsChannel 短信渠道：一个渠道对应一组云厂商凭证（租户隔离）。
// Config 里存 access_key / access_secret / sign_name 等；密钥只落库不进代码，
// 对外返回时由 handler 层做脱敏。
type SmsChannel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	TenantID  uint           `gorm:"not null;default:1;index" json:"tenant_id"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	Provider  string         `gorm:"size:32;not null" json:"provider"` // debug|aliyun|tencent
	Config    map[string]any `gorm:"column:config;type:json;serializer:json" json:"config"`
	Status    int8           `gorm:"default:1" json:"status"` // 1 启用 / 0 停用
	Remark    string         `gorm:"size:255" json:"remark"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func (SmsChannel) TableName() string {
	return "sms_channels"
}

// SmsTemplate 短信模板：code 租户内唯一；content 含 {name} 形式占位；
// ProviderTemplateID 对应云厂商侧的模板号（阿里 TemplateCode / 腾讯 TemplateID）。
type SmsTemplate struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	TenantID           uint      `gorm:"not null;default:1;index" json:"tenant_id"`
	Code               string    `gorm:"size:100;not null" json:"code"`
	Name               string    `gorm:"size:100;not null" json:"name"`
	ChannelID          uint      `gorm:"index" json:"channel_id"`
	Content            string    `gorm:"type:text;not null" json:"content"`
	Type               int8      `gorm:"default:1" json:"type"`
	ProviderTemplateID string    `gorm:"size:100" json:"provider_template_id"`
	Status             int8      `gorm:"default:1" json:"status"`
	Remark             string    `gorm:"size:255" json:"remark"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (SmsTemplate) TableName() string {
	return "sms_templates"
}

// SmsLog 发送日志：记录一次发送的手机号、渲染后内容、入参、渠道快照与结果。
type SmsLog struct {
	ID            uint              `gorm:"primaryKey" json:"id"`
	TenantID      uint              `gorm:"not null;default:1;index" json:"tenant_id"`
	Mobile        string            `gorm:"size:32;index" json:"mobile"`
	TemplateCode  string            `gorm:"size:100;index" json:"template_code"`
	Content       string            `gorm:"type:text" json:"content"`
	Params        map[string]string `gorm:"column:params;type:json;serializer:json" json:"params"`
	ChannelID     uint              `json:"channel_id"`
	ChannelName   string            `gorm:"size:100" json:"channel_name"`
	Provider      string            `gorm:"size:32" json:"provider"`
	Status        string            `gorm:"size:16;index" json:"status"` // sending|success|failure
	ProviderMsgID string            `gorm:"size:128" json:"provider_msg_id"`
	Error         string            `gorm:"size:512" json:"error"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func (SmsLog) TableName() string {
	return "sms_logs"
}
