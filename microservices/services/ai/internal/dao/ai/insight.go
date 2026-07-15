package ai

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// InsightDAO aggregates login_logs and operation_logs for AI-generated
// security reports. It reads the shared audit tables directly with plain SQL
// so the AI service stays independent from the audit service.
type InsightDAO struct {
	db *gorm.DB
}

// NewInsightDAO builds an InsightDAO backed by an injected handle.
func NewInsightDAO(db *gorm.DB) *InsightDAO {
	return &InsightDAO{db: db}
}

func (d *InsightDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

// NameCount is one aggregation bucket.
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// LoginStats aggregates login_logs over a time window.
type LoginStats struct {
	Total          int64       `json:"total"`
	Success        int64       `json:"success"`
	Failed         int64       `json:"failed"`
	UniqueIPs      int64       `json:"unique_ips"`
	FailureReasons []NameCount `json:"failure_reasons"`
}

// OperationStats aggregates operation_logs over a time window.
type OperationStats struct {
	Total     int64       `json:"total"`
	ByModule  []NameCount `json:"by_module"`
	ByActor   []NameCount `json:"by_actor"`
	ErrorRows int64       `json:"error_rows"`
}

// LoginStatsSinceContext aggregates login_logs created at or after since.
func (d *InsightDAO) LoginStatsSinceContext(ctx context.Context, since time.Time) (*LoginStats, error) {
	stats := &LoginStats{}
	db := d.dbWithContext(ctx)

	row := struct {
		Total     int64
		Success   int64
		Failed    int64
		UniqueIPs int64 `gorm:"column:unique_ips"`
	}{}
	if err := db.Raw(
		`SELECT COUNT(*) AS total,
		        COUNT(*) FILTER (WHERE status = 1) AS success,
		        COUNT(*) FILTER (WHERE status <> 1) AS failed,
		        COUNT(DISTINCT ip) AS unique_ips
		 FROM login_logs WHERE created_at >= $1`, since,
	).Scan(&row).Error; err != nil {
		return nil, err
	}
	stats.Total = row.Total
	stats.Success = row.Success
	stats.Failed = row.Failed
	stats.UniqueIPs = row.UniqueIPs

	if err := db.Raw(
		`SELECT message AS name, COUNT(*) AS count
		 FROM login_logs
		 WHERE created_at >= $1 AND status <> 1 AND message <> ''
		 GROUP BY message ORDER BY count DESC LIMIT 5`, since,
	).Scan(&stats.FailureReasons).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// OperationStatsSinceContext aggregates operation_logs created at or after
// since.
func (d *InsightDAO) OperationStatsSinceContext(ctx context.Context, since time.Time) (*OperationStats, error) {
	stats := &OperationStats{}
	db := d.dbWithContext(ctx)

	row := struct {
		Total     int64
		ErrorRows int64 `gorm:"column:error_rows"`
	}{}
	if err := db.Raw(
		`SELECT COUNT(*) AS total,
		        COUNT(*) FILTER (WHERE status >= 400) AS error_rows
		 FROM operation_logs WHERE created_at >= $1`, since,
	).Scan(&row).Error; err != nil {
		return nil, err
	}
	stats.Total = row.Total
	stats.ErrorRows = row.ErrorRows

	if err := db.Raw(
		`SELECT module AS name, COUNT(*) AS count
		 FROM operation_logs
		 WHERE created_at >= $1 AND module <> ''
		 GROUP BY module ORDER BY count DESC LIMIT 5`, since,
	).Scan(&stats.ByModule).Error; err != nil {
		return nil, err
	}

	if err := db.Raw(
		`SELECT username AS name, COUNT(*) AS count
		 FROM operation_logs
		 WHERE created_at >= $1 AND username <> ''
		 GROUP BY username ORDER BY count DESC LIMIT 5`, since,
	).Scan(&stats.ByActor).Error; err != nil {
		return nil, err
	}
	return stats, nil
}
