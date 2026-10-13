package localmodel

import "time"

// Hermes M5：Cron 定时任务。控制面定义 cron 任务（cron 表达式 + 提示词），
// manager 在设备端评估调度、到点通过 loopback API server 推提示词给本地 Hermes，
// 结果回传控制面落 hermes_cron_runs 审计。
//
// 设计决策：不依赖未验证的上游 `hermes cron`；v1 仅在 company_cluster 设备执行
// （loopback API server 只在该 profile 下发）；员工 company 设备收到 Cron 元数据但 no-op。

// HermesCronActionPrompt 是 v1 唯一动作：把提示词推给本地 Hermes 网关聊天接口。
const HermesCronActionPrompt = "prompt"

const (
	HermesCronRunStatusTriggered = "triggered"
	HermesCronRunStatusCompleted = "completed"
	HermesCronRunStatusFailed    = "failed"
)

// HermesCronJob 是租户级定时任务定义。tenant_id 作用域由租户插件自动加。
type HermesCronJob struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint      `gorm:"not null;index;uniqueIndex:uk_hermes_cron_jobs_tenant_name,priority:1" json:"tenant_id"`
	Name           string    `gorm:"size:80;not null;uniqueIndex:uk_hermes_cron_jobs_tenant_name,priority:2" json:"name"`
	Description    string    `gorm:"type:text;not null" json:"description"`
	CronExpression string    `gorm:"size:40;not null" json:"cron_expression"`
	ActionType     string    `gorm:"size:16;not null" json:"action_type"`
	Prompt         string    `gorm:"type:text;not null" json:"prompt"`
	Enabled        bool      `gorm:"not null" json:"enabled"`
	CreatedBy      uint      `gorm:"not null" json:"created_by"`
	UpdatedBy      uint      `gorm:"not null" json:"updated_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName maps HermesCronJob onto hermes_cron_jobs.
func (HermesCronJob) TableName() string { return "hermes_cron_jobs" }

// HermesCronRun 是单次设备端触发记录（审计）。由设备经签名通道写入，控制面只读列表。
type HermesCronRun struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	TenantID    uint       `gorm:"not null;index" json:"tenant_id"`
	JobID       uint64     `gorm:"not null;index:idx_hermes_cron_runs_tenant_job,priority:2" json:"job_id"`
	DeviceID    string     `gorm:"size:64;not null;index:idx_hermes_cron_runs_tenant_device,priority:2" json:"device_id"`
	Status      string     `gorm:"size:16;not null" json:"status"`
	Result      string     `gorm:"type:text;not null" json:"result"`
	TriggeredAt *time.Time `json:"triggered_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TableName maps HermesCronRun onto hermes_cron_runs.
func (HermesCronRun) TableName() string { return "hermes_cron_runs" }
