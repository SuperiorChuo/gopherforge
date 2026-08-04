package monitor

import (
	"context"
	"errors"
	"time"

	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
	"github.com/go-admin-kit/server/internal/model"
)

var ErrInvalidTrendRange = errors.New("invalid trend range")

// MetricTrendResponse is the downsampled series returned by the trend API.
type MetricTrendResponse struct {
	Metric string                  `json:"metric"`
	Range  string                  `json:"range"`
	Unit   string                  `json:"unit"`
	Points []model.MetricTrendPoint `json:"points"`
}

type trendRangeConfig struct {
	window time.Duration
	bucket time.Duration
}

var trendRangeConfigs = map[string]trendRangeConfig{
	"1h":  {window: time.Hour, bucket: time.Minute},
	"24h": {window: 24 * time.Hour, bucket: 5 * time.Minute},
	"7d":  {window: 7 * 24 * time.Hour, bucket: time.Hour},
}

// MetricTrendService reads sampled history and downsamples it into a fixed
// number of points for the range requested (1h / 24h / 7d).
type MetricTrendService struct {
	store monitordao.MetricSampleStore
	now   func() time.Time
}

func NewMetricTrendService(store monitordao.MetricSampleStore) *MetricTrendService {
	return &MetricTrendService{store: store, now: time.Now}
}

func (s *MetricTrendService) QueryTrendContext(ctx context.Context, metric, rng string) (MetricTrendResponse, error) {
	cfg, ok := trendRangeConfigs[rng]
	if !ok {
		return MetricTrendResponse{}, ErrInvalidTrendRange
	}
	if !isKnownAlertMetric(metric) {
		return MetricTrendResponse{}, ErrInvalidAlertMetric
	}
	now := s.now().UTC()
	from := now.Add(-cfg.window)
	raw, err := s.store.QueryRawContext(ctx, metric, from, now)
	if err != nil {
		return MetricTrendResponse{}, err
	}
	points := downsampleTrendPoints(raw, cfg.bucket)
	return MetricTrendResponse{Metric: metric, Range: rng, Unit: alertMetricUnit(metric), Points: points}, nil
}

// downsampleTrendPoints buckets raw samples and averages each bucket, so the
// returned series stays small regardless of range.
func downsampleTrendPoints(raw []model.MonitorMetricSample, bucket time.Duration) []model.MetricTrendPoint {
	if len(raw) == 0 {
		return []model.MetricTrendPoint{}
	}
	points := make([]model.MetricTrendPoint, 0, len(raw)/(int(bucket)/int(time.Minute))+1)
	var current *trendBucket
	for _, sample := range raw {
		ts := sample.CollectedAt.Truncate(bucket)
		if current == nil || !current.ts.Equal(ts) {
			if current != nil {
				points = append(points, current.finish())
			}
			current = &trendBucket{ts: ts}
		}
		current.add(sample.Value)
	}
	if current != nil {
		points = append(points, current.finish())
	}
	return points
}

type trendBucket struct {
	ts  time.Time
	sum float64
	n   int
}

func (b *trendBucket) add(value float64) {
	b.sum += value
	b.n++
}

func (b *trendBucket) finish() model.MetricTrendPoint {
	return model.MetricTrendPoint{Timestamp: b.ts.UnixMilli(), Value: b.sum / float64(b.n)}
}
