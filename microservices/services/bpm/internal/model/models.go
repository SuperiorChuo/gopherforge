// Package model 定义 bpm M1 五张核心表：流程定义（版本化）/ 流程实例 /
// 审批任务 / 抄送记录 / 操作日志。全部含 tenant_id 隔离，主键 uint64，
// JSON 字段以 JSONB 自定义类型落库（postgres=jsonb，sqlite 测试=text）。
package model

import (
	"database/sql/driver"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ---------- JSONB 自定义类型 ----------

// JSONB 承载原样 JSON（node_tree / form_snapshot / variables / detail）。
type JSONB []byte

// Value 落库：空值按 "{}" 写，满足 NOT NULL DEFAULT '{}' 语义。
func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	return string(j), nil
}

// Scan 读库：兼容 []byte / string。
func (j *JSONB) Scan(v any) error {
	switch s := v.(type) {
	case nil:
		*j = nil
		return nil
	case []byte:
		*j = append((*j)[0:0], s...)
		return nil
	case string:
		*j = append((*j)[0:0], s...)
		return nil
	default:
		return errors.New("JSONB: 不支持的扫描类型")
	}
}

// MarshalJSON 序列化输出原样 JSON（空值输出 null）。
func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON 原样保留输入 JSON。
func (j *JSONB) UnmarshalJSON(b []byte) error {
	*j = append((*j)[0:0], b...)
	return nil
}

// GormDBDataType 按方言选择列类型：postgres 用 jsonb，其余（sqlite 测试）用 text。
func (JSONB) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "postgres" {
		return "jsonb"
	}
	return "text"
}

// ---------- 状态枚举 ----------

// 定义状态。
const (
	DefDraft     = "draft"
	DefActive    = "active"
	DefSuspended = "suspended"
	DefArchived  = "archived"
)

// 实例状态（suspended：审批人解析为空且兜底策略为挂起时的非终态）。
const (
	InstRunning   = "running"
	InstApproved  = "approved"
	InstRejected  = "rejected"
	InstCanceled  = "canceled"
	InstSuspended = "suspended"
)

// 任务状态。
const (
	TaskPending  = "pending"
	TaskApproved = "approved"
	TaskRejected = "rejected"
	TaskCanceled = "canceled"
	TaskSkipped  = "skipped"
	TaskReturned = "returned" // M2 退回启用
)

// ---------- 流程定义（版本化） ----------

type ProcessDefinition struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_bpm_def_key_ver,priority:1;index" json:"tenant_id"`
	Key      string `gorm:"size:64;not null;uniqueIndex:ux_bpm_def_key_ver,priority:2" json:"key"`
	Version  int    `gorm:"not null;default:1;uniqueIndex:ux_bpm_def_key_ver,priority:3" json:"version"`
	Name     string `gorm:"size:128;not null" json:"name"`
	// Status: draft|active|suspended|archived；应用层保证同 (tenant,key) 至多一条 active
	Status   string `gorm:"size:16;not null;default:draft;index" json:"status"`
	NodeTree JSONB  `gorm:"not null" json:"node_tree"`
	// FormSchema 可选：发起表单字段声明（M1 可空，业务方自持表单）
	FormSchema JSONB     `json:"form_schema,omitempty"`
	BizType    string    `gorm:"size:32;index" json:"biz_type"`
	Remark     string    `gorm:"size:256" json:"remark,omitempty"`
	CreatedBy  uint64    `gorm:"not null;default:0" json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ProcessDefinition) TableName() string { return "bpm_process_definition" }

// ---------- 流程实例 ----------

type ProcessInstance struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;index:ix_bpm_inst_tenant,priority:1;index:ix_bpm_inst_initiator,priority:1" json:"tenant_id"`
	// DefinitionID 冻结到发起时的具体版本行；定义再发新版不影响在途实例
	DefinitionID  uint64 `gorm:"not null" json:"definition_id"`
	DefinitionKey string `gorm:"size:64;not null;index" json:"definition_key"`
	Title         string `gorm:"size:256;not null" json:"title"`
	BizType       string `gorm:"size:32;not null" json:"biz_type"`
	// BizID 字符串承载业务主键，引擎不假设类型
	BizID  string `gorm:"size:64;not null" json:"biz_id"`
	Status string `gorm:"size:16;not null;default:running;index:ix_bpm_inst_tenant,priority:2;index:ix_bpm_inst_initiator,priority:3" json:"status"`
	// CurrentNodeID 单游标：当前推进到的节点 id（node_tree 内 id）；终态清空
	CurrentNodeID string `gorm:"size:64" json:"current_node_id"`
	// FormSnapshot 发起时冻结的表单快照（条件求值与展示依据）
	FormSnapshot JSONB `gorm:"not null" json:"form_snapshot"`
	// Variables 运行期变量（M1：self_select 的选人结果 selected_assignees）
	Variables     JSONB      `json:"variables"`
	InitiatorID   uint64     `gorm:"not null;index:ix_bpm_inst_initiator,priority:2" json:"initiator_id"`
	InitiatorDept uint64     `gorm:"not null;default:0" json:"initiator_dept"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (ProcessInstance) TableName() string { return "bpm_process_instance" }

// ---------- 审批任务（待办） ----------

type Task struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	TenantID   uint64 `gorm:"not null;default:1;index:ix_bpm_task_todo,priority:1" json:"tenant_id"`
	InstanceID uint64 `gorm:"not null;index:ix_bpm_task_inst,priority:1" json:"instance_id"`
	NodeID     string `gorm:"size:64;not null;index:ix_bpm_task_inst,priority:2" json:"node_id"`
	NodeName   string `gorm:"size:128;not null" json:"node_name"`
	// Round 退回重审时同节点的第几轮（M1 恒 1，M2 退回启用）
	Round      int    `gorm:"not null;default:1;index:ix_bpm_task_inst,priority:3" json:"round"`
	AssigneeID uint64 `gorm:"not null;index:ix_bpm_task_todo,priority:2" json:"assignee_id"`
	// OriginAssignee 转办前的原处理人（0=未转办；M2 转办启用）
	OriginAssignee uint64 `gorm:"not null;default:0" json:"origin_assignee,omitempty"`
	// MultiMode 冗余自节点（AND|OR|SEQ），便于收敛判定与查询
	MultiMode string `gorm:"size:8;not null;default:OR" json:"multi_mode"`
	// SeqOrder SEQ 模式下的顺位（M3 启用）
	SeqOrder int    `gorm:"not null;default:0" json:"seq_order"`
	Status   string `gorm:"size:16;not null;default:pending;index:ix_bpm_task_todo,priority:3" json:"status"`
	Comment  string `gorm:"size:512" json:"comment,omitempty"`
	// TimeoutAt 超时提醒时间点（M2 ticker 启用，M1 仅落库）
	TimeoutAt  *time.Time `json:"timeout_at,omitempty"`
	RemindedAt *time.Time `json:"reminded_at,omitempty"`
	ActedAt    *time.Time `json:"acted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (Task) TableName() string { return "bpm_task" }

// ---------- 抄送记录 ----------

type CcRecord struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	TenantID   uint64     `gorm:"not null;default:1;index:ix_bpm_cc_user,priority:1" json:"tenant_id"`
	InstanceID uint64     `gorm:"not null;index" json:"instance_id"`
	NodeID     string     `gorm:"size:64;not null" json:"node_id"`
	UserID     uint64     `gorm:"not null;index:ix_bpm_cc_user,priority:2" json:"user_id"`
	ReadAt     *time.Time `json:"read_at,omitempty"` // NULL=未读（已读接口 M2）
	CreatedAt  time.Time  `json:"created_at"`
}

func (CcRecord) TableName() string { return "bpm_cc_record" }

// ---------- 操作 / 流转日志 ----------

// 日志动作（M1 使用的子集；transfer/return_*/timeout_remind 留 M2）。
const (
	ActionSubmit         = "submit"
	ActionApprove        = "approve"
	ActionReject         = "reject"
	ActionCancel         = "cancel"
	ActionCc             = "cc"
	ActionAutoPass       = "auto_pass"
	ActionSuspend        = "suspend"
	ActionFinishApproved = "finish_approved"
	ActionFinishRejected = "finish_rejected"
)

type ProcessLog struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	TenantID   uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	InstanceID uint64 `gorm:"not null;index:ix_bpm_log_inst,priority:1" json:"instance_id"`
	// NodeID 系统级动作（发起/终态）可为空
	NodeID string `gorm:"size:64" json:"node_id,omitempty"`
	TaskID uint64 `gorm:"not null;default:0" json:"task_id,omitempty"`
	Action string `gorm:"size:32;not null" json:"action"`
	// OperatorID 0=系统
	OperatorID uint64 `gorm:"not null;default:0" json:"operator_id"`
	// Detail 附加信息：意见、抄送对象、挂起原因等
	Detail    JSONB     `json:"detail,omitempty"`
	CreatedAt time.Time `gorm:"index:ix_bpm_log_inst,priority:2" json:"created_at"`
}

func (ProcessLog) TableName() string { return "bpm_process_log" }
