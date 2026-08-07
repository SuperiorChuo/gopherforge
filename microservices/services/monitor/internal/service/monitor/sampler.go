package monitor

import (
	"context"
	"strings"
	"sync"
	"time"

	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	"github.com/go-admin-kit/services/monitor/internal/model"
)

// DefaultSamplingInterval is how often the background sampler records one
// point per monitored metric.
const DefaultSamplingInterval = 60 * time.Second

// DefaultSampleRetention is how long sampled history is kept before pruning.
const DefaultSampleRetention = 7 * 24 * time.Hour

// pruneEveryRuns prunes retention roughly every 30 runs (~30 min at 60s).
const pruneEveryRuns = 30

// MetricSampler periodically samples every alert metric (same collector and
// metric keys as the alert engine) and persists one point per metric to
// monitor_metric_samples, feeding the trend API. A single failing metric never
// fails a batch — the rest still land.
type MetricSampler struct {
	collector AlertMetricCollector
	store     monitordao.MetricSampleStore
	interval  time.Duration
	retention time.Duration

	done chan struct{}
	once sync.Once
}

func NewMetricSampler(collector AlertMetricCollector, store monitordao.MetricSampleStore, interval, retention time.Duration) *MetricSampler {
	if interval <= 0 {
		interval = DefaultSamplingInterval
	}
	if retention <= 0 {
		retention = DefaultSampleRetention
	}
	return &MetricSampler{
		collector: collector,
		store:     store,
		interval:  interval,
		retention: retention,
		done:      make(chan struct{}),
	}
}

// StartMetricSampler launches the sampling loop in the background; stop it
// with Shutdown. Mirrors StartAlertEvaluator's lifecycle.
func StartMetricSampler(parent context.Context, collector AlertMetricCollector, store monitordao.MetricSampleStore, interval time.Duration) *MetricSampler {
	sampler := NewMetricSampler(collector, store, interval, DefaultSampleRetention)
	go sampler.run(parent)
	return sampler
}

func (s *MetricSampler) Shutdown(ctx context.Context) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *MetricSampler) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	s.collectOnce(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	runs := 0
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			runs++
			s.collectOnce(ctx)
			if runs%pruneEveryRuns == 0 {
				s.prune(ctx)
			}
		}
	}
}

func (s *MetricSampler) collectOnce(ctx context.Context) {
	definitions := ListAlertMetrics()
	if len(definitions) == 0 {
		return
	}
	now := time.Now().UTC()
	samples := make([]model.MonitorMetricSample, 0, len(definitions))
	for _, definition := range definitions {
		value, err := s.collector.CollectContext(ctx, definition.Key)
		if err != nil {
			continue
		}
		samples = append(samples, model.MonitorMetricSample{
			Category:    metricCategory(definition.Key),
			Metric:      definition.Key,
			Value:       value,
			CollectedAt: now,
		})
	}
	if len(samples) == 0 {
		return
	}
	_ = s.store.InsertBatchContext(ctx, samples)
}

func (s *MetricSampler) prune(ctx context.Context) {
	before := time.Now().UTC().Add(-s.retention)
	_, _ = s.store.PruneBeforeContext(ctx, before)
}

func metricCategory(metric string) string {
	switch {
	case strings.HasPrefix(metric, "system."):
		return "system"
	case strings.HasPrefix(metric, "postgres."):
		return "postgres"
	case strings.HasPrefix(metric, "redis."):
		return "redis"
	}
	return "other"
}
