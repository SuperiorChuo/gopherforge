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
