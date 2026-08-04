package monitor

import (
	"context"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/gorm"
)

// MetricSampleStore abstracts persistence of monitor metric samples. The
// sampler writes them, the trend service reads them.
type MetricSampleStore interface {
	InsertBatchContext(ctx context.Context, samples []model.MonitorMetricSample) error
	QueryRawContext(ctx context.Context, metric string, from, to time.Time) ([]model.MonitorMetricSample, error)
	PruneBeforeContext(ctx context.Context, before time.Time) (int64, error)
}

type MetricSampleDAO struct {
	db *gorm.DB
}

func NewMetricSampleDAO(db *gorm.DB) *MetricSampleDAO {
	return &MetricSampleDAO{db: db}
}

func (d *MetricSampleDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *MetricSampleDAO) InsertBatchContext(ctx context.Context, samples []model.MonitorMetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	return d.dbWithContext(ctx).Create(&samples).Error
}

func (d *MetricSampleDAO) QueryRawContext(ctx context.Context, metric string, from, to time.Time) ([]model.MonitorMetricSample, error) {
	var samples []model.MonitorMetricSample
	err := d.dbWithContext(ctx).
		Where("metric = ? AND collected_at >= ? AND collected_at <= ?", metric, from, to).
		Order("collected_at ASC").
		Find(&samples).Error
	return samples, err
}

func (d *MetricSampleDAO) PruneBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	result := d.dbWithContext(ctx).
		Where("collected_at < ?", before).
		Delete(&model.MonitorMetricSample{})
	return result.RowsAffected, result.Error
}
