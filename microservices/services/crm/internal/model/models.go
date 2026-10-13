// Package model defines the sales-pipeline CRM entities (CRM-M1):
// 线索 → 客户/联系人 → 商机（阶段流转）→ 合同 → 回款，外加通用跟进记录。
// 金额一律存"分"（int64），避免浮点精度问题；展示层 /100。
package model

import "time"

// ---------- 线索 ----------

// LeadStatus: new(未处理) following(跟进中) converted(已转化) invalid(无效)
type Lead struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	Company  string `gorm:"size:128" json:"company"`
	Phone    string `gorm:"size:32;index" json:"phone"`
	Email    string `gorm:"size:128" json:"email"`
	// Source: manual | call | im | import
	Source      string `gorm:"size:32;not null;default:manual" json:"source"`
	Status      string `gorm:"size:32;not null;default:new;index" json:"status"`
	OwnerUserID uint64 `gorm:"index" json:"owner_user_id"`
	// DepartmentID 名片分到部门池（P1）：owner=0 且 dept>0 时部门成员可领取。
	DepartmentID uint64 `gorm:"not null;default:0;index" json:"department_id"`
	// IMVisitorID 结构化关联 IM 访客（P4 对话历史入口）。
	IMVisitorID uint64    `gorm:"not null;default:0;index" json:"im_visitor_id,omitempty"`
	Note        string    `gorm:"type:text" json:"note,omitempty"`
	CustomerID  *uint64   `json:"customer_id,omitempty"`           // 转化后回填
	Score       int       `gorm:"not null;default:0" json:"score"` // 线索评分 0-100
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Lead) TableName() string { return "crm_leads" }

// RecalcScore 根据线索质量自动计算评分（0-100）。来源质量 + 信息完整度。
func (l *Lead) RecalcScore() {
	s := 0
	switch l.Source {
	case "referral":
		s += 30
	case "call":
		s += 25
	case "im":
		s += 20
	case "import":
		s += 15
	case "manual":
		s += 10
	default:
		s += 5
	}
	if l.Phone != "" {
		s += 20
	}
	if l.Company != "" {
		s += 20
	}
	if l.Email != "" {
		s += 10
	}
	if l.Note != "" {
		s += 10
	}
	if time.Since(l.CreatedAt) < 7*24*time.Hour {
		s += 10
	}
	l.Score = s
}

// ---------- 客户与联系人 ----------

type Customer struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	Industry string `gorm:"size:64" json:"industry"`
	// Level: A | B | C（重要程度）
	Level       string `gorm:"size:8;default:C" json:"level"`
	Phone       string `gorm:"size:32;index" json:"phone"`
	Address     string `gorm:"size:256" json:"address"`
	Source      string `gorm:"size:32;not null;default:manual" json:"source"`
	OwnerUserID uint64 `gorm:"index" json:"owner_user_id"`
	// IMVisitorID 由名片转化时继承（P4 对话历史入口）。
	IMVisitorID uint64 `gorm:"not null;default:0;index" json:"im_visitor_id,omitempty"`
	// 公海引擎（P2）：LastFollowAt 跟进/领取时刷新；owner=0 即公海，
	// PooledAt 记录入池时间。
	LastFollowAt *time.Time `json:"last_follow_at,omitempty"`
	PooledAt     *time.Time `json:"pooled_at,omitempty"`
	Note         string     `gorm:"type:text" json:"note,omitempty"`
	Tags         []Tag      `gorm:"many2many:crm_customer_tags;" json:"tags,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (Customer) TableName() string { return "crm_customers" }

type Contact struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	TenantID   uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	CustomerID uint64    `gorm:"not null;index" json:"customer_id"`
	Name       string    `gorm:"size:64;not null" json:"name"`
	Title      string    `gorm:"size:64" json:"title"`
	Phone      string    `gorm:"size:32" json:"phone"`
	Email      string    `gorm:"size:128" json:"email"`
	IsPrimary  bool      `gorm:"not null;default:false" json:"is_primary"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Contact) TableName() string { return "crm_contacts" }

// ---------- 商机 ----------

// 阶段与默认赢率；won/lost 为终态。
var Stages = []string{"initial", "demand", "proposal", "negotiation", "won", "lost"}

var StageProbability = map[string]int{
	"initial": 10, "demand": 30, "proposal": 50, "negotiation": 80, "won": 100, "lost": 0,
}

func ValidStage(s string) bool {
	for _, v := range Stages {
		if v == s {
			return true
		}
	}
	return false
}

type Opportunity struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	TenantID   uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	CustomerID uint64 `gorm:"not null;index" json:"customer_id"`
	Name       string `gorm:"size:128;not null" json:"name"`
	// AmountCents 预计成交金额（分）
	AmountCents     int64      `gorm:"not null;default:0" json:"amount_cents"`
	Stage           string     `gorm:"size:32;not null;default:initial;index" json:"stage"`
	Probability     int        `gorm:"not null;default:10" json:"probability"`
	ExpectedCloseAt *time.Time `json:"expected_close_at,omitempty"`
	OwnerUserID     uint64     `gorm:"index" json:"owner_user_id"`
	LostReason      string     `gorm:"size:256" json:"lost_reason,omitempty"`
	WonAt           *time.Time `json:"won_at,omitempty"`
	LostAt          *time.Time `json:"lost_at,omitempty"`
	Note            string     `gorm:"type:text" json:"note,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Opportunity) TableName() string { return "crm_opportunities" }

// OpportunityStageLog 记录每次阶段推进，销售过程可回溯。
type OpportunityStageLog struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	OpportunityID uint64    `gorm:"not null;index" json:"opportunity_id"`
	FromStage     string    `gorm:"size:32" json:"from_stage"`
	ToStage       string    `gorm:"size:32;not null" json:"to_stage"`
	Note          string    `gorm:"size:512" json:"note,omitempty"`
	ByUserID      uint64    `json:"by_user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func (OpportunityStageLog) TableName() string { return "crm_opportunity_stage_logs" }

// ---------- 合同与回款 ----------

// ContractStatus: draft(草拟) approving(审批中,BPM_ENABLE 开启后使用) active(生效) void(作废)
// 状态机（BPM 开启时）：draft → approving → active | 回 draft；BPM 关闭时与历史行为一致。
type Contract struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	TenantID      uint64     `gorm:"not null;default:1;uniqueIndex:ux_crm_contract_no,priority:1;index" json:"tenant_id"`
	No            string     `gorm:"size:64;not null;uniqueIndex:ux_crm_contract_no,priority:2" json:"no"`
	Title         string     `gorm:"size:128;not null" json:"title"`
	CustomerID    uint64     `gorm:"not null;index" json:"customer_id"`
	OpportunityID *uint64    `gorm:"index" json:"opportunity_id,omitempty"`
	AmountCents   int64      `gorm:"not null;default:0" json:"amount_cents"`
	Status        string     `gorm:"size:32;not null;default:draft" json:"status"`
	SignedAt      *time.Time `json:"signed_at,omitempty"`
	StartAt       *time.Time `json:"start_at,omitempty"`
	EndAt         *time.Time `json:"end_at,omitempty"`
	OwnerUserID   uint64     `gorm:"index" json:"owner_user_id"`
	Note          string     `gorm:"type:text" json:"note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (Contract) TableName() string { return "crm_contracts" }

type Payment struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	ContractID  uint64    `gorm:"not null;index" json:"contract_id"`
	AmountCents int64     `gorm:"not null" json:"amount_cents"`
	PaidAt      time.Time `json:"paid_at"`
	// Method: transfer | cash | other
	Method string `gorm:"size:32;default:transfer" json:"method"`
	Note   string `gorm:"size:256" json:"note,omitempty"`
	// Status: pending_approval(审批中) | confirmed(已确认) | rejected(已驳回)。
	// BPM M2 回款审批：BPM_ENABLE 开启后登记即 pending_approval，审批通过才
	// confirmed（此后才计入回款统计）；关闭时直接 confirmed，与接入前一致。
	// 列默认 confirmed：加列时存量历史回款自动视为已确认（设计文档 §8-Q4），
	// 启动补数兜底空值，见 store.NewWithDB。
	Status    string    `gorm:"size:16;not null;default:confirmed;index" json:"status"`
	ByUserID  uint64    `json:"by_user_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (Payment) TableName() string { return "crm_payments" }

// ---------- 分配/公海留痕（P1/P2） ----------

// AssignLog 记录名片与客户的每次归属变化：assign(管理分配) claim(领取)
// release(释放公海) recycle(引擎自动回收)。
type AssignLog struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	TenantID   uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	TargetType string    `gorm:"size:16;not null;index:idx_crm_assign_target,priority:1" json:"target_type"` // lead | customer
	TargetID   uint64    `gorm:"not null;index:idx_crm_assign_target,priority:2" json:"target_id"`
	Action     string    `gorm:"size:16;not null" json:"action"`
	FromOwner  uint64    `json:"from_owner"`
	ToOwner    uint64    `json:"to_owner"`
	ToDept     uint64    `json:"to_dept"`
	ByUserID   uint64    `json:"by_user_id"`
	Note       string    `gorm:"size:256" json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (AssignLog) TableName() string { return "crm_assign_logs" }

// PoolRule 公海回收规则引擎（P2）：租户内按客户级别配置回收天数；
// level="*" 为默认档，A/B/C 覆盖默认。IdleDays=0 表示该档不回收。
type PoolRule struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_crm_pool_rule,priority:1" json:"tenant_id"`
	Level    string `gorm:"size:8;not null;default:*;uniqueIndex:ux_crm_pool_rule,priority:2" json:"level"`
	// IdleDays 超过该天数未跟进则回收公海。
	IdleDays int `gorm:"not null;default:30" json:"idle_days"`
	// ProtectActive 为 true 时，有进行中商机或生效合同的客户不自动回收。
	ProtectActive bool      `gorm:"not null;default:true" json:"protect_active"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	UpdatedBy     uint64    `json:"updated_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (PoolRule) TableName() string { return "crm_pool_rules" }

// ---------- 跟进记录 ----------

// FollowUp 挂在 lead / customer / opportunity 任一目标上。
type FollowUp struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	TenantID   uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	TargetType string `gorm:"size:16;not null;index:idx_crm_followups_target,priority:1" json:"target_type"`
	TargetID   uint64 `gorm:"not null;index:idx_crm_followups_target,priority:2" json:"target_id"`
	Content    string `gorm:"type:text;not null" json:"content"`
	// NextAt 下次跟进提醒时间（可空）
	NextAt    *time.Time `json:"next_at,omitempty"`
	ByUserID  uint64     `json:"by_user_id"`
	CreatedAt time.Time  `json:"created_at"`
}

func (FollowUp) TableName() string { return "crm_followups" }
