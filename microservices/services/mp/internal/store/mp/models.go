// Package store persists 公众号 entities. AutoMigrate 实验线（与 im/cc/crm 同约定），
// 全部查询强制 tenant_id 隔离。真实密钥不入库：mp_accounts 只存引用名（SecretRef/
// AESKeyRef），明文走 .env 或 system_settings。
package store

import "time"

// MpAccount 公众号账号配置。AppID 全局唯一（回调按 :appid 路由，跨租户唯一）。
type MpAccount struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	AppID    string `gorm:"size:64;uniqueIndex:ux_mp_accounts_appid;not null" json:"app_id"`
	Name     string `gorm:"size:128" json:"name"`
	// Type: service(服务号) | subscribe(订阅号)。决定客服/网页授权等接口是否可用。
	Type string `gorm:"size:16;not null;default:service" json:"type"`
	// GhID 原始 ID（gh_ 开头），被动回复 XML 的 FromUserName。
	GhID string `gorm:"size:64" json:"gh_id"`
	// SecretRef/AESKeyRef 存引用名（env 变量名或 system_settings key），非明文。
	SecretRef string `gorm:"size:128" json:"secret_ref"`
	Token     string `gorm:"size:128" json:"-"` // 回调 Token；响应不回显
	AESKeyRef string `gorm:"size:128" json:"aes_key_ref"`
	// EncryptMode: plain(明文) | compat(兼容) | safe(安全/AES)。
	EncryptMode string    `gorm:"size:16;not null;default:plain" json:"encrypt_mode"`
	AutoPublish bool      `gorm:"not null;default:false" json:"auto_publish"`
	Status      string    `gorm:"size:16;not null;default:active" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MpAccount) TableName() string { return "mp_accounts" }

// MpMessage 收发消息留档。RawBody 全存原始 XML/JSON，便于排查与回放。
type MpMessage struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index;uniqueIndex:ux_mp_messages_inbound_msg,priority:1,where:direction = 'in' AND msg_id <> ''" json:"tenant_id"`
	AccountID uint64 `gorm:"not null;index:idx_mp_messages_account;uniqueIndex:ux_mp_messages_inbound_msg,priority:2,where:direction = 'in' AND msg_id <> ''" json:"account_id"`
	OpenID    string `gorm:"size:64;index:idx_mp_messages_openid" json:"open_id"`
	// Direction: in(粉丝→公众号) | out(公众号→粉丝)。
	Direction string    `gorm:"size:8;not null" json:"direction"`
	MsgType   string    `gorm:"size:16;not null" json:"msg_type"`
	Content   string    `gorm:"type:text" json:"content"`
	MsgID     string    `gorm:"size:64;index;uniqueIndex:ux_mp_messages_inbound_msg,priority:3,where:direction = 'in' AND msg_id <> ''" json:"msg_id"` // 微信 MsgId，幂等去重
	Event     string    `gorm:"size:32" json:"event"`                                                                                                  // subscribe/CLICK/PUBLISHJOBFINISH 等
	RawBody   string    `gorm:"type:text" json:"raw_body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (MpMessage) TableName() string { return "mp_messages" }

// MpPublishResultJob is a durable inbox for verified PUBLISHJOBFINISH events.
// It closes the race where WeChat's callback arrives before the local publish
// request has persisted its publish_id.
type MpPublishResultJob struct {
	ID            uint64 `gorm:"primaryKey" json:"id"`
	TenantID      uint64 `gorm:"not null;default:1;uniqueIndex:ux_mp_publish_result_job,priority:1" json:"tenant_id"`
	AccountID     uint64 `gorm:"not null;uniqueIndex:ux_mp_publish_result_job,priority:2" json:"account_id"`
	PublishID     string `gorm:"size:128;not null;uniqueIndex:ux_mp_publish_result_job,priority:3" json:"publish_id"`
	PublishStatus string `gorm:"size:16;not null" json:"publish_status"`
	// Status: pending | settled | dead.
	Status    string     `gorm:"size:16;not null;default:pending;index:idx_mp_publish_result_due,priority:1" json:"status"`
	Attempts  int        `gorm:"not null;default:0" json:"attempts"`
	NextAt    time.Time  `gorm:"not null;index:idx_mp_publish_result_due,priority:2" json:"next_at"`
	LastError string     `gorm:"size:500" json:"last_error"`
	SettledAt *time.Time `json:"settled_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (MpPublishResultJob) TableName() string { return "mp_publish_result_jobs" }

// MpFanSyncJob tracks one full fan synchronization run. ActiveKey is set only
// while the job is pending/running, so the unique index prevents concurrent
// runs for the same tenant/account while preserving terminal history.
type MpFanSyncJob struct {
	ID        uint64  `gorm:"primaryKey" json:"id"`
	TenantID  uint64  `gorm:"not null;default:1;index" json:"-"`
	AccountID uint64  `gorm:"not null;index" json:"account_id"`
	ActiveKey *string `gorm:"size:64;uniqueIndex:ux_mp_fan_sync_active" json:"-"`
	// Status: pending | running | succeeded | failed.
	Status string `gorm:"size:16;not null;default:pending;index:idx_mp_fan_sync_due,priority:1" json:"status"`
	// Cursor is the cursor used to load the next page. PageOpenIDs and
	// PageOffset persist the current page so a restarted worker resumes at the
	// next 100-user batch instead of fetching or recounting the page.
	Cursor      string     `gorm:"size:128" json:"-"`
	NextCursor  string     `gorm:"size:128" json:"-"`
	PageOpenIDs string     `gorm:"type:text" json:"-"`
	PageOffset  int        `gorm:"not null;default:0" json:"-"`
	TotalCount  int        `gorm:"not null;default:0" json:"total_count"`
	ListedCount int        `gorm:"not null;default:0" json:"listed_count"`
	SyncedCount int        `gorm:"not null;default:0" json:"synced_count"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	NextAt      time.Time  `gorm:"not null;index:idx_mp_fan_sync_due,priority:2" json:"next_at"`
	LeaseUntil  *time.Time `gorm:"index" json:"-"`
	LeaseToken  string     `gorm:"size:64" json:"-"`
	LastError   string     `gorm:"size:500" json:"last_error,omitempty"`
	CreatedBy   uint64     `gorm:"not null;default:0" json:"created_by"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (MpFanSyncJob) TableName() string { return "mp_fan_sync_jobs" }

// MpFan 公众号粉丝档案。本阶段（M3）表与列表接口先落地，粉丝数据 M4 才从
// 微信用户接口拉取；(AccountID, OpenID) 联合唯一，同一账号下 openid 不重复。
type MpFan struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	AccountID uint64 `gorm:"index;uniqueIndex:ux_mp_fans_acc_openid,priority:1" json:"account_id"`
	OpenID    string `gorm:"size:64;uniqueIndex:ux_mp_fans_acc_openid,priority:2" json:"open_id"`
	Nickname  string `gorm:"size:128" json:"nickname"`
	Avatar    string `gorm:"size:512" json:"avatar"`
	Sex       int8   `gorm:"" json:"sex"`
	City      string `gorm:"size:64" json:"city"`
	Country   string `gorm:"size:64" json:"country"`
	Province  string `gorm:"size:64" json:"province"`
	Tags      string `gorm:"size:256" json:"tags"`
	// Subscribed 关注中；取关事件把它置 false（M4）。
	Subscribed    bool       `gorm:"not null;default:true" json:"subscribed"`
	SubscribeTime *time.Time `json:"subscribe_time,omitempty"`
	// SubscribeEventAt is the effective version of the subscription state. A
	// callback is placed at the last PostgreSQL microsecond of its second so it
	// wins over a concurrent API snapshot.
	SubscribeEventAt *time.Time `json:"-"`
	LastSyncJobID    uint64     `gorm:"not null;default:0;index" json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// AccountID 需与 OpenID 组成联合唯一索引 ux_mp_fans_acc_openid。
func (MpFan) TableName() string { return "mp_fans" }

// MpMenu 自定义菜单草稿。Buttons 存微信 button JSON；同一租户下每个账号一条。
type MpMenu struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index;uniqueIndex:ux_mp_menus_tenant_account,priority:1" json:"tenant_id"`
	AccountID uint64    `gorm:"index;uniqueIndex:ux_mp_menus_tenant_account,priority:2" json:"account_id"`
	Buttons   string    `gorm:"type:text" json:"buttons"`
	Published bool      `gorm:"not null;default:false" json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MpMenu) TableName() string { return "mp_menus" }
