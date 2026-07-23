package model

import "time"

// ScheduledJob stores scheduled task definitions.
type ScheduledJob struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:100;not null;uniqueIndex" json:"name"`
	GroupName      string     `gorm:"size:50;default:default" json:"group_name"`
	CronExpression string     `gorm:"size:50;not null" json:"cron_expression"`
	InvokeTarget   string     `gorm:"size:255;not null" json:"invoke_target"`
	Description    string     `gorm:"size:500" json:"description"`
	Status         int8       `gorm:"default:1" json:"status"`
	Concurrent     int8       `gorm:"default:0" json:"concurrent"`
	LastRunTime    *time.Time `json:"last_run_time"`
	NextRunTime    *time.Time `json:"next_run_time"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (ScheduledJob) TableName() string {
	return "scheduled_jobs"
}

// ScheduledJobLog stores scheduled task execution logs.
type ScheduledJobLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	JobID     uint      `gorm:"not null;index" json:"job_id"`
	JobName   string    `gorm:"size:100;not null" json:"job_name"`
	Status    int8      `gorm:"default:1" json:"status"`
	Message   string    `gorm:"type:text" json:"message"`
	Duration  int       `json:"duration"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (ScheduledJobLog) TableName() string {
	return "scheduled_job_logs"
}

// OpsJobHeartbeat is a distributed job heartbeat (task center M1): in-process
// loops across services and host shell crons report one row per run (via
// shared/pkg/jobbeat or psql upsert), covering the silent-failure blind spot
// that scheduled_jobs (monitor's in-process cron) cannot see.
// The table is created by migration 000026; monitor only reads and aggregates.
type OpsJobHeartbeat struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	JobKey         string    `gorm:"size:100;uniqueIndex" json:"job_key"`
	Service        string    `gorm:"size:50" json:"service"`
	Description    string    `gorm:"size:255" json:"description"`
	IntervalSec    int64     `json:"interval_sec"`
	LastRunAt      time.Time `json:"last_run_at"`
	LastStatus     string    `gorm:"size:16" json:"last_status"`
	LastError      string    `gorm:"type:text" json:"last_error"`
	LastDurationMS int64     `gorm:"column:last_duration_ms" json:"last_duration_ms"`
	Runs           int64     `json:"runs"`
	Fails          int64     `json:"fails"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (OpsJobHeartbeat) TableName() string {
	return "ops_job_heartbeats"
}
