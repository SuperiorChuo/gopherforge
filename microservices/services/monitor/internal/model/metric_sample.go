package localmodel

import "time"

// MonitorMetricSample is one point of a monitored metric sampled by the
// monitor service's background sampler. Values share the same metric keys as
// the alert rule engine (see alert_metric.go) so trends and alerts are one
// source of truth.
type MonitorMetricSample struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Category    string    `gorm:"size:32;not null" json:"category"`
	Metric      string    `gorm:"size:64;not null;index" json:"metric"`
	Value       float64   `gorm:"not null" json:"value"`
	CollectedAt time.Time `gorm:"not null" json:"collected_at"`
}

func (MonitorMetricSample) TableName() string {
	return "monitor_metric_samples"
}

// MetricTrendPoint is one downsampled point of a trend series.
type MetricTrendPoint struct {
	Timestamp int64   `json:"t"`
	Value     float64 `json:"value"`
}
