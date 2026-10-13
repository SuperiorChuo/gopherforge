// Package content 是社媒管理的平台无关内容中台：一条内容(Post)可发布到多个渠道
// (PostPublication)。渠道专属逻辑在 internal/publisher/<channel> 里实现，本包只管
// 内容与发布记录的存储，不含任何微信/小红书等平台细节。
//
// 与 store 包（微信专属 mp_* 表）并存：微信回调/素材/草稿仍在 store，本包是其上
// 的抽象层。新内容走 Post，存量 MpArticle 只读兼容。
package content

import "time"

// Channel 渠道标识。与 publisher.Channel 对应（避免包循环，这里用 string 常量）。
const (
	ChannelWechat      = "wechat"
	ChannelWeibo       = "weibo"
	ChannelXiaohongshu = "xiaohongshu"
	ChannelZhihu       = "zhihu"
	ChannelDouyin      = "douyin"
	ChannelBilibili    = "bilibili"
)

// MediaItem 内容里的一张图/一段视频。URL 可为 http 或 data:base64（AI 生图）。
type MediaItem struct {
	Kind string `json:"kind"` // image | video
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// Post 平台无关的内容。一次 AI 生成或人工撰写产出一个 Post；针对不同平台的风格
// 变体各自是独立 Post（便于分别编辑/发布），用 Platform 标注它面向的目标平台。
type Post struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index:idx_posts_tenant_status,priority:1" json:"tenant_id"`

	Title    string `gorm:"size:256" json:"title"`
	BodyHTML string `gorm:"type:text" json:"body_html"`
	Summary  string `gorm:"size:512" json:"summary"`
	CoverURL string `gorm:"type:text" json:"cover_url"` // http 或 data:base64
	// Media 附加图片/视频，JSON 序列化存储。
	Media MediaItemList `gorm:"type:text" json:"media"`
	// Tags 话题/标签（小红书 #话题#、微博话题等），JSON 数组。
	Tags StringList `gorm:"type:text" json:"tags"`

	// Platform 目标平台风格（wechat/xiaohongshu/...）；空=通用。仅影响 AI 生成风格
	// 与导出文案格式，不限制能发到哪些渠道。
	Platform string `gorm:"size:32" json:"platform"`

	// Status: draft(草稿) | ready(待发布) | archived。发布状态在 PostPublication 上。
	Status string `gorm:"size:32;not null;default:draft;index:idx_posts_tenant_status,priority:2" json:"status"`

	AIPrompt  string    `gorm:"type:text" json:"ai_prompt"`
	CreatedBy uint64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Post) TableName() string { return "posts" }

// PostPublication 一条内容到某渠道某账号的一次发布记录。一 Post → 多 Publication。
type PostPublication struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index:idx_pub_tenant_post,priority:1" json:"tenant_id"`
	PostID   uint64 `gorm:"not null;index:idx_pub_tenant_post,priority:2" json:"post_id"`

	Channel   string `gorm:"size:32;not null;index" json:"channel"` // wechat/weibo/...
	AccountID uint64 `gorm:"index" json:"account_id"`               // 渠道账号 id（如 mp_accounts.id）

	// Status: pending → (publishing | exporting) → published | exported | failed
	Status string `gorm:"size:32;not null;default:pending;index" json:"status"`
	// Mode: api(直发) | export(半自动导出)。由 adapter 能力决定。
	Mode string `gorm:"size:16" json:"mode"`

	// 平台侧结果
	ExternalID  string `gorm:"size:128;index" json:"external_id"` // 微信 publish_id / 微博 mid 等
	ExternalURL string `gorm:"type:text" json:"external_url"`
	Error       string `gorm:"type:text" json:"error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PostPublication) TableName() string { return "post_publications" }

// ChannelAccount 泛化的渠道账号：一个租户在某渠道下的一个可发布账号。平台专属配置
// （微信的 appid/token、微博的 access_token 引用等）存 ConfigJSON。密钥仍只存引用名。
//
// 与 store.MpAccount 的关系：微信渠道下，ChannelAccount 通过 RefID 指向 mp_accounts.id
// 复用现有微信配置；其他渠道 RefID=0，配置全在 ConfigJSON。避免搬迁存量微信账号。
type ChannelAccount struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	Channel  string `gorm:"size:32;not null;index" json:"channel"`
	Name     string `gorm:"size:128" json:"name"`
	// RefID 指向渠道自有账号表主键（微信=mp_accounts.id）；为 0 则配置全在 ConfigJSON。
	RefID uint64 `gorm:"index" json:"ref_id"`
	// ConfigJSON 平台专属配置（密钥只存引用名）。
	ConfigJSON string    `gorm:"type:text" json:"config_json,omitempty"`
	Status     string    `gorm:"size:16;not null;default:active" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ChannelAccount) TableName() string { return "channel_accounts" }
