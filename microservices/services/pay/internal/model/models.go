// Package model 定义支付中心数据表（AutoMigrate 实验线，照 ticket/crm 范式）。
// 金额一律以「分」(int64) 存储；全表 tenant_id 隔离。
package model

import "time"

// 订单状态机：pending 仅逻辑态（创建即 waiting）——
// waiting ──拉起支付──► paying ──渠道回调成功──► paid（终态）
//
//	│                    │
//	└──超时/主动关单──► closed（终态）◄──────────┘
const (
	OrderWaiting = "waiting"
	OrderPaying  = "paying"
	OrderPaid    = "paid"
	OrderClosed  = "closed"
)

// 退款状态机：pending ──提交渠道──► processing ──回调──► refunded / failed（均终态）
const (
	RefundPending    = "pending"
	RefundProcessing = "processing"
	RefundRefunded   = "refunded"
	RefundFailed     = "failed"
)

// Channel 渠道配置。敏感凭据 M1 只有 mock 渠道无凭据；真实渠道（M2）凭据
// AES-GCM 加密后入 CredentialsEnc，读取一律脱敏（见设计文档 §6）。
type Channel struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_pay_channels_tenant_code,priority:1;index" json:"tenant_id"`
	// Code 渠道类型：mock / wechat_native / wechat_jsapi / alipay_pc / alipay_wap
	Code string `gorm:"size:32;not null;uniqueIndex:ux_pay_channels_tenant_code,priority:2" json:"code"`
	Name string `gorm:"size:100;not null" json:"name"`
	// Status 1=启用 2=停用
	Status int8 `gorm:"not null;default:1" json:"status"`
	// FeeRateBps 渠道费率（万分位，仅展示/对账参考）
	FeeRateBps int `gorm:"not null;default:0" json:"fee_rate_bps"`
	// CredentialsEnc 加密后的渠道凭据 JSON（M1 恒空；json:"-" 永不回显）
	CredentialsEnc string `gorm:"type:text" json:"-"`
	// KeyVersion 主密钥版本（密钥轮换预留，设计 §6）
	KeyVersion int       `gorm:"not null;default:0" json:"key_version"`
	Remark     string    `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Channel) TableName() string { return "pay_channels" }

// Order 支付订单。业务幂等锚点：(tenant_id, biz_type, biz_id) 唯一——
// 同一业务单重复创建返回既有未终态订单。
type Order struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_pay_orders_biz,priority:1;index" json:"tenant_id"`
	OrderNo  string `gorm:"size:32;not null;uniqueIndex" json:"order_no"`
	// OrderToken 收银台匿名访问凭据（随机；终态后失效）
	OrderToken string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	BizType    string `gorm:"size:32;not null;uniqueIndex:ux_pay_orders_biz,priority:2" json:"biz_type"`
	BizID      string `gorm:"size:64;not null;uniqueIndex:ux_pay_orders_biz,priority:3" json:"biz_id"`
	Subject    string `gorm:"size:255;not null" json:"subject"`
	// AmountCents 应收金额（分，服务端定价，创建后不可改）
	AmountCents int64 `gorm:"not null" json:"amount_cents"`
	// RefundedCents 累计已退（分），事务内校验 ≤ AmountCents
	RefundedCents int64  `gorm:"not null;default:0" json:"refunded_cents"`
	Status        string `gorm:"size:16;not null;default:'waiting';index" json:"status"`
	// ChannelID/ChannelCode 用户在收银台选定的渠道（waiting 期可换）
	ChannelID   uint64 `gorm:"not null;default:0" json:"channel_id"`
	ChannelCode string `gorm:"size:32" json:"channel_code"`
	// ChannelTradeNo 渠道侧交易号（paid 时回填）
	ChannelTradeNo string     `gorm:"size:64;index" json:"channel_trade_no"`
	PaidAt         *time.Time `json:"paid_at"`
	// ExpireAt 过期关单时间（补偿 cron 扫描）
	ExpireAt  time.Time `gorm:"not null;index" json:"expire_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Order) TableName() string { return "pay_orders" }

// Refund 退款单。同一订单可多次部分退款，累计 ≤ 实收。
type Refund struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	RefundNo string `gorm:"size:32;not null;uniqueIndex" json:"refund_no"`
	OrderID  uint64 `gorm:"not null;index" json:"order_id"`
	OrderNo  string `gorm:"size:32;not null;index" json:"order_no"`
	// AmountCents 本次退款金额（分）
	AmountCents int64  `gorm:"not null" json:"amount_cents"`
	Reason      string `gorm:"size:255" json:"reason"`
	Status      string `gorm:"size:16;not null;default:'pending';index" json:"status"`
	// ChannelRefundNo 渠道侧退款单号
	ChannelRefundNo string `gorm:"size:64" json:"channel_refund_no"`
	// OperatorUserID 发起人（管理面）或 0（internal 端点）
	OperatorUserID uint64     `gorm:"not null;default:0" json:"operator_user_id"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Refund) TableName() string { return "pay_refunds" }

// NotifyLog 渠道回调原始日志：验签结论与处理结果，审计与差错排查用。
type NotifyLog struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	// Kind trade=支付回调 refund=退款回调
	Kind        string `gorm:"size:16;not null" json:"kind"`
	ChannelCode string `gorm:"size:32;not null" json:"channel_code"`
	OrderNo     string `gorm:"size:32;index" json:"order_no"`
	// Body 回调原文（截断至 8KB）
	Body string `gorm:"type:text" json:"body"`
	// SignOK 验签是否通过
	SignOK bool `gorm:"not null;default:false" json:"sign_ok"`
	// Result 处理结论：applied / duplicate / amount_mismatch / order_not_found / rejected
	Result    string    `gorm:"size:32;not null" json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

func (NotifyLog) TableName() string { return "pay_notify_logs" }

// CallbackJob 业务终态回调任务：成功即删，失败留存重试（指数退避），
// 耗尽后 status=dead 可管理面手工重推。
type CallbackJob struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	OrderID  uint64 `gorm:"not null;index" json:"order_id"`
	// Kind order=支付终态 refund=退款终态
	Kind     string `gorm:"size:16;not null" json:"kind"`
	RefundID uint64 `gorm:"not null;default:0" json:"refund_id"`
	// Attempts 已尝试次数；NextAt 下次可执行时间（SKIP LOCKED 抢单）
	Attempts int       `gorm:"not null;default:0" json:"attempts"`
	NextAt   time.Time `gorm:"not null;index" json:"next_at"`
	// Status pending / dead
	Status    string    `gorm:"size:16;not null;default:'pending';index" json:"status"`
	LastError string    `gorm:"size:500" json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CallbackJob) TableName() string { return "pay_callback_jobs" }
