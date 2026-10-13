package model

import (
	"time"

	"github.com/google/uuid"
)

type Site struct {
	ID             uint64 `gorm:"primaryKey" json:"id"`
	TenantID       uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	AppKey         string `gorm:"size:64;uniqueIndex;not null" json:"app_key"`
	AppSecret      string `gorm:"size:128;not null" json:"-"`
	Name           string `gorm:"size:128;not null" json:"name"`
	AllowedOrigins string `gorm:"type:text" json:"allowed_origins"` // JSON 数组字符串
	WelcomeText    string `gorm:"type:text" json:"welcome_text"`
	Status         int16  `gorm:"default:1" json:"status"`
	// BotEnabled：为 true 时新会话从 bot_serving 开始接待（M4）。
	BotEnabled bool `gorm:"default:true" json:"bot_enabled"`
	// BotSystemPrompt 可选：AI 机器人的站点级系统提示词。
	BotSystemPrompt string `gorm:"type:text" json:"bot_system_prompt,omitempty"`
	// BotKBEnabled：为 true 时机器人回复优先检索 ai_documents（RAG M1）。
	// 默认 true，使既有站点在迁移后即可获得知识检索能力。
	BotKBEnabled bool `gorm:"not null;default:true" json:"bot_kb_enabled"`
	// DefaultSkillGroupID：访客未指定技能组时，用它为新会话路由。
	DefaultSkillGroupID *uint64 `json:"default_skill_group_id,omitempty"`
	// BusinessHours JSON（M7）：{"enabled":bool,"days":[1..7],"start":"09:00",
	// "end":"18:00","off_text":"..."}；空值 = 全天候。
	BusinessHours string    `gorm:"type:text" json:"business_hours,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Site) TableName() string { return "im_sites" }

type Visitor struct {
	ID          uint64  `gorm:"primaryKey" json:"id"`
	SiteID      uint64  `gorm:"uniqueIndex:ux_site_guest;not null" json:"site_id"`
	GuestKey    string  `gorm:"size:64;uniqueIndex:ux_site_guest;not null" json:"guest_key"`
	UserID      *uint64 `json:"user_id,omitempty"`
	DisplayName string  `gorm:"size:128" json:"display_name"`
	// Phone 是访客在 widget / H5 里留下的联系电话，或由坐席补录。
	Phone string `gorm:"size:32" json:"phone,omitempty"`
	// Email / Wechat：对话识别或坐席补录的联系方式。
	Email  string `gorm:"size:128" json:"email,omitempty"`
	Wechat string `gorm:"size:64" json:"wechat,omitempty"`
	Meta   string `gorm:"type:text" json:"meta,omitempty"`
	// Note 是坐席维护的访客备注（M5）。
	Note string `gorm:"type:text" json:"note,omitempty"`
	// 被禁访客无法开启会话或发送消息（M6）。
	Banned     bool      `gorm:"not null;default:false" json:"banned"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Visitor) TableName() string { return "im_visitors" }

// SkillGroup 负责队列分配的路由。
type SkillGroup struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_im_skill_groups_tenant_code,priority:1;index" json:"tenant_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	Code     string `gorm:"size:64;uniqueIndex:ux_im_skill_groups_tenant_code,priority:2;not null" json:"code"`
	// 策略：round_robin | least_load | manual
	Strategy string `gorm:"size:32;not null;default:round_robin" json:"strategy"`
	Status   int16  `gorm:"default:1" json:"status"`
	// RRCursor 是 round-robin 最后一次分配的 agent_user_id（尽力而为）。
	RRCursor  uint64    `gorm:"default:0" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SkillGroup) TableName() string { return "im_skill_groups" }

// AgentSkill 将后端用户绑定到技能组。
type AgentSkill struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	AgentUserID   uint64    `gorm:"uniqueIndex:ux_agent_skill;index;not null" json:"agent_user_id"`
	SkillGroupID  uint64    `gorm:"uniqueIndex:ux_agent_skill;index;not null" json:"skill_group_id"`
	MaxConcurrent int       `gorm:"default:5;not null" json:"max_concurrent"`
	Status        int16     `gorm:"default:1" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (AgentSkill) TableName() string { return "im_agent_skills" }

// AgentPresence 由 PG 存储（供 M3 使用；后续可用 Redis 替代）。
type AgentPresence struct {
	AgentUserID uint64 `gorm:"primaryKey" json:"agent_user_id"`
	// 状态：online | busy | offline
	Status      string    `gorm:"size:16;not null;default:offline;index" json:"status"`
	DisplayName string    `gorm:"size:128" json:"display_name"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentPresence) TableName() string { return "im_agent_presence" }

type Conversation struct {
	ID       uint64    `gorm:"primaryKey" json:"id"`
	PublicID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"public_id"`
	// tenant_id/status/agent_user_id 的查询索引由 store.ensureIndexes 维护
	// （复合 + partial，gorm tag 表达不了），这里不再声明单列索引。
	TenantID     uint64  `gorm:"not null;default:1" json:"tenant_id"`
	SiteID       uint64  `gorm:"index;not null" json:"site_id"`
	Channel      string  `gorm:"size:32;not null;default:h5" json:"channel"`
	VisitorID    uint64  `gorm:"index;not null" json:"visitor_id"`
	AgentUserID  *uint64 `json:"agent_user_id,omitempty"`
	SkillGroupID *uint64 `gorm:"index" json:"skill_group_id,omitempty"`
	Status       string  `gorm:"size:32;not null;default:queued" json:"status"`
	Context      string  `gorm:"type:text" json:"context,omitempty"`
	CloseReason  string  `gorm:"size:64" json:"close_reason,omitempty"`
	// Summary 是 AI/规则生成的会话摘要（M4）。
	Summary            string     `gorm:"type:text" json:"summary,omitempty"`
	QueuedAt           *time.Time `json:"queued_at,omitempty"`
	AssignedAt         *time.Time `json:"assigned_at,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview string     `gorm:"size:256" json:"last_message_preview,omitempty"`
	// LastSeq 是本会话的消息 seq 计数器：CreateMessage 通过一次原子
	// UPDATE ... RETURNING 递增它，因此 seq 分配既不需要 SELECT MAX(seq)，
	// 也不需要显式行锁。迁移时已从 MAX(im_messages.seq) 为存量行回填。
	LastSeq int64 `gorm:"not null;default:0" json:"last_seq"`
	// 读游标：双方各自已读的最后消息 seq（0 = 未读任何消息）。
	AgentLastReadSeq   int64 `gorm:"not null;default:0" json:"agent_last_read_seq"`
	VisitorLastReadSeq int64 `gorm:"not null;default:0" json:"visitor_last_read_seq"`
	// 会话关闭后收集的访客 CSAT 评分（M5）：1-5，nil = 未评分。
	RatingScore   *int16     `json:"rating_score,omitempty"`
	RatingComment string     `gorm:"size:512" json:"rating_comment,omitempty"`
	RatedAt       *time.Time `json:"rated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// QueuePosition 是访客读取时为排队中会话计算的排队位置（M6）。
	QueuePosition int64 `gorm:"-" json:"queue_position,omitempty"`
}

func (Conversation) TableName() string { return "im_conversations" }

type Message struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// (conversation_id, seq) 唯一索引由 store.ensureIndexes 维护
	ConversationID uint64    `gorm:"not null" json:"conversation_id"`
	ClientMsgID    *string   `gorm:"size:64" json:"client_msg_id,omitempty"`
	SenderType     string    `gorm:"size:16;not null" json:"sender_type"`
	SenderID       *uint64   `json:"sender_id,omitempty"`
	MsgType        string    `gorm:"size:32;not null;default:text" json:"msg_type"`
	Content        string    `gorm:"type:text;not null" json:"content"` // JSON 字符串
	Seq            int64     `gorm:"not null" json:"seq"`
	CreatedAt      time.Time `json:"created_at"`
	// Replayed 标记一次幂等的 client_msg_id 重发：该行已存在，
	// 因此调用方必须跳过 hub 广播 / 机器人副作用。
	Replayed bool `gorm:"-" json:"-"`
}

func (Message) TableName() string { return "im_messages" }

// QuickReply 是面向坐席工作台的租户共享快捷回复（M5）。
type QuickReply struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	Title     string    `gorm:"size:64;not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	CreatedBy uint64    `gorm:"not null;default:0" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (QuickReply) TableName() string { return "im_quick_replies" }

// SensitiveWord 是租户级敏感词条目，由控制台维护
// (DB 词库热生效；env IM_SENSITIVE_WORDS 仍作为进程级兜底,仅打码).
type SensitiveWord struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_im_sensitive_tenant_word,priority:1" json:"tenant_id"`
	Word     string `gorm:"size:128;not null;uniqueIndex:ux_im_sensitive_tenant_word,priority:2" json:"word"`
	// Action: mask（命中打码为 *）| block（拒绝发送）
	Action string `gorm:"size:16;not null;default:mask" json:"action"`
	// Tag 分类标签,如 辱骂/广告/涉政,仅用于管理端筛选
	Tag       string    `gorm:"size:64" json:"tag,omitempty"`
	Status    int16     `gorm:"not null;default:1" json:"status"`
	CreatedBy uint64    `gorm:"not null;default:0" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SensitiveWord) TableName() string { return "im_sensitive_words" }

// KBMiss 记录知识检索未能回答的访客问题
// （供运营后续补充知识库）。
type KBMiss struct {
	ID                   uint64 `gorm:"primaryKey" json:"id"`
	TenantID             uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	SiteID               uint64 `gorm:"not null;default:0;index" json:"site_id"`
	ConversationPublicID string `gorm:"size:64;index" json:"conversation_public_id"`
	VisitorID            uint64 `gorm:"not null;default:0;index" json:"visitor_id"`
	Question             string `gorm:"type:text;not null" json:"question"`
	// 状态：open | resolved | ignored
	Status    string    `gorm:"size:16;not null;default:open;index" json:"status"`
	Note      string    `gorm:"type:text" json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KBMiss) TableName() string { return "im_kb_misses" }

// KBDailyStat 是知识检索（M1.5）的按租户按日计数器。
type KBDailyStat struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	TenantID     uint64 `gorm:"not null;uniqueIndex:ux_im_kb_stat_day,priority:1" json:"tenant_id"`
	Day          string `gorm:"size:10;not null;uniqueIndex:ux_im_kb_stat_day,priority:2" json:"day"` // YYYY-MM-DD
	Hits         int64  `gorm:"not null;default:0" json:"hits"`
	Misses       int64  `gorm:"not null;default:0" json:"misses"`
	VectorHits   int64  `gorm:"not null;default:0" json:"vector_hits"`
	KeywordHits  int64  `gorm:"not null;default:0" json:"keyword_hits"`
	VectorErrors int64  `gorm:"not null;default:0" json:"vector_errors"`
}

func (KBDailyStat) TableName() string { return "im_kb_daily_stats" }
