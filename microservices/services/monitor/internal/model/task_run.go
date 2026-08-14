package localmodel

import "time"

const (
	TaskRunStatusRunning   = "running"
	TaskRunStatusSucceeded = "succeeded"
	TaskRunStatusFailed    = "failed"
	TaskRunStatusCancelled = "cancelled"
)

// OpsTaskRun is one explicitly tracked operational task execution. Poll-loop
// liveness stays in OpsJobHeartbeat and is intentionally not duplicated here.
type OpsTaskRun struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	RunID         string     `gorm:"size:64;not null;uniqueIndex" json:"run_id"`
	TaskKey       string     `gorm:"size:100;not null;index" json:"task_key"`
	Service       string     `gorm:"size:50;not null;index" json:"service"`
	Description   string     `gorm:"size:255;not null" json:"description"`
	Source        string     `gorm:"size:24;not null" json:"source"`
	TriggerType   string     `gorm:"size:24;not null" json:"trigger_type"`
	Status        string     `gorm:"size:16;not null;index" json:"status"`
	Attempt       int        `gorm:"not null" json:"attempt"`
	CorrelationID string     `gorm:"size:96;not null" json:"correlation_id"`
	StartedAt     time.Time  `gorm:"not null;index" json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	DurationMS    int64      `gorm:"not null" json:"duration_ms"`
	Message       string     `gorm:"type:text;not null" json:"message"`
	ErrorMessage  string     `gorm:"type:text;not null" json:"error_message"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
}

func (OpsTaskRun) TableName() string {
	return "ops_task_runs"
}
