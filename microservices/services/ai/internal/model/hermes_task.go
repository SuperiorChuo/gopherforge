package localmodel

import "time"

// Hermes 第二期：多 Agent 协作任务。仅控制面编排，动物理链路在下一里程碑。
//
// 三个已拍板决策（详见 docs/hermes-multi-agent-phase2.md）：
//   1) 任务正文明文落库（description/result 均为服务端明文，供拆解/审批/审计）；
//   2) company_cluster 策略层开放 dispatcher（delegation/skills/mcp）；
//   3) 本期不动物理链路：clients/hermes-manager 零改动，company_cluster 策略
//      「不可应用到真实设备」，由服务层守卫 + buildDesiredState 防御性拒绝兜底。

const (
	// HermesProfileCompany 是 P1 默认策略 profile（manager isExactP1Toolset
	// 只收 P1 四件套，因此该 profile 是唯一可下发到设备的形态）。
	HermesProfileCompany = "company"
	// HermesProfileCompanyCluster 开放 dispatcher 的策略 profile。注意：本期
	// manager 验签器不接受其工具集，该 profile 的策略只能用于集群/任务的
	// 创建、指派与追踪，不能应用到任何真实设备（见服务层守卫）。
	HermesProfileCompanyCluster = "company_cluster"
)

const (
	HermesTaskStatusPending   = "pending"
	HermesTaskStatusActive    = "active"
	HermesTaskStatusCompleted = "completed"
	HermesTaskStatusCancelled = "cancelled"
)

const (
	HermesTaskRunStatusAssigned  = "assigned"
	HermesTaskRunStatusRunning   = "running"
	HermesTaskRunStatusCompleted = "completed"
	HermesTaskRunStatusFailed    = "failed"
)

// HermesCluster 是 company_cluster 分组：把若干逻辑 Agent 实例归为一组，共享
// 一个 dispatcher 策略（policy_id），供多 Agent 协作任务派发。policy_id=0
// 表示集群尚未绑定 dispatcher 策略。
// Slug 非空 = 该集群启用 kanban 通道（每集群一 board，board 目录名 = slug）；
// 空 = 既有 push 通道。
type HermesCluster struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint      `gorm:"not null;index;uniqueIndex:uk_hermes_clusters_tenant_name,priority:1" json:"tenant_id"`
	Name        string    `gorm:"size:80;not null;uniqueIndex:uk_hermes_clusters_tenant_name,priority:2" json:"name"`
	Slug        string    `gorm:"size:80;not null" json:"slug"`
	Description string    `gorm:"type:text;not null" json:"description"`
	PolicyID    uint64    `gorm:"not null" json:"policy_id"`
	Enabled     bool      `gorm:"not null" json:"enabled"`
	CreatedBy   uint      `gorm:"not null" json:"created_by"`
	UpdatedBy   uint      `gorm:"not null" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (HermesCluster) TableName() string { return "hermes_clusters" }

// HermesTask 是任务定义。Description 即任务正文（决策 1：服务端明文存储）。
// ClusterID 指向目标集群；PolicyID 为 0 时沿用集群策略。status 流转：
// pending → active → completed | cancelled。
type HermesTask struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	TenantID    uint       `gorm:"not null;index" json:"tenant_id"`
	Name        string     `gorm:"size:120;not null" json:"name"`
	Description string     `gorm:"type:text;not null" json:"description"`
	Status      string     `gorm:"size:16;not null" json:"status"`
	ClusterID   uint64     `gorm:"not null" json:"cluster_id"`
	PolicyID    uint64     `gorm:"not null" json:"policy_id"`
	CreatedBy   uint       `gorm:"not null" json:"created_by"`
	UpdatedBy   uint       `gorm:"not null" json:"updated_by"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (HermesTask) TableName() string { return "hermes_tasks" }

// HermesTaskRun 是单实例执行记录。Status：assigned → running → completed | failed。
// Result 是结果摘要（明文）。uk_hermes_task_runs_task_instance 阻止同一任务对
// 同一实例的重复指派。
type HermesTaskRun struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	TenantID   uint       `gorm:"not null;index" json:"tenant_id"`
	TaskID     uint64     `gorm:"not null;uniqueIndex:uk_hermes_task_runs_task_instance,priority:1" json:"task_id"`
	InstanceID uint64     `gorm:"not null;uniqueIndex:uk_hermes_task_runs_task_instance,priority:2;index" json:"instance_id"`
	Status     string     `gorm:"size:16;not null" json:"status"`
	Result     string     `gorm:"type:text;not null" json:"result"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedBy  uint       `gorm:"not null" json:"created_by"`
	UpdatedBy  uint       `gorm:"not null" json:"updated_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (HermesTaskRun) TableName() string { return "hermes_task_runs" }
