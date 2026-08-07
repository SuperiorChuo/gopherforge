package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/model"
)

type fakeAlertMetricCollector struct {
	values map[string]float64
	errs   map[string]error
}

func (f *fakeAlertMetricCollector) CollectContext(ctx context.Context, metric string) (float64, error) {
	if err := f.errs[metric]; err != nil {
		return 0, err
	}
	return f.values[metric], nil
}

type fakeMetricSampleStore struct {
	inserted    []model.MonitorMetricSample
	raw         []model.MonitorMetricSample
	pruned      bool
	prunedBefore time.Time
}

func (f *fakeMetricSampleStore) InsertBatchContext(_ context.Context, samples []model.MonitorMetricSample) error {
	f.inserted = append(f.inserted, samples...)
	return nil
}

func (f *fakeMetricSampleStore) QueryRawContext(_ context.Context, _ string, _, _ time.Time) ([]model.MonitorMetricSample, error) {
	return f.raw, nil
}

func (f *fakeMetricSampleStore) PruneBeforeContext(_ context.Context, before time.Time) (int64, error) {
	f.pruned = true
	f.prunedBefore = before
	return 0, nil
}

func allValues(v float64) map[string]float64 {
	values := make(map[string]float64, len(alertMetricDefinitions))
	for _, definition := range alertMetricDefinitions {
		values[definition.Key] = v
	}
	return values
}

func TestMetricSamplerCollectOnceBatches(t *testing.T) {
	collector := &fakeAlertMetricCollector{values: allValues(42)}
	store := &fakeMetricSampleStore{}
	sampler := NewMetricSampler(collector, store, time.Minute, DefaultSampleRetention)
	sampler.collectOnce(context.Background())

	if got := len(store.inserted); got != len(alertMetricDefinitions) {
		t.Fatalf("inserted %d samples, want %d", got, len(alertMetricDefinitions))
	}
	for _, sample := range store.inserted {
		if sample.Value != 42 {
			t.Errorf("sample %s value = %v, want 42", sample.Metric, sample.Value)
		}
		if sample.Category == "" {
			t.Errorf("sample %s has empty category", sample.Metric)
		}
		if sample.CollectedAt.IsZero() {
			t.Errorf("sample %s has zero collected_at", sample.Metric)
		}
	}
}

func TestMetricSamplerSkipsFailedCollect(t *testing.T) {
	values := make(map[string]float64)
	fails := make(map[string]error)
	for i, definition := range alertMetricDefinitions {
		if i == 0 {
			fails[definition.Key] = errors.New("boom")
			continue
		}
		values[definition.Key] = 1
	}
	collector := &fakeAlertMetricCollector{values: values, errs: fails}
	store := &fakeMetricSampleStore{}
	sampler := NewMetricSampler(collector, store, time.Minute, DefaultSampleRetention)
	sampler.collectOnce(context.Background())

	if got := len(store.inserted); got != len(alertMetricDefinitions)-1 {
		t.Fatalf("inserted %d samples, want %d", got, len(alertMetricDefinitions)-1)
	}
	for _, sample := range store.inserted {
		if sample.Metric == alertMetricDefinitions[0].Key {
			t.Fatalf("failed metric %s was still inserted", sample.Metric)
		}
	}
}

func TestMetricSamplerPrunesRetention(t *testing.T) {
	collector := &fakeAlertMetricCollector{values: allValues(1)}
	store := &fakeMetricSampleStore{}
	sampler := NewMetricSampler(collector, store, time.Minute, 24*time.Hour)
	sampler.prune(context.Background())
	if !store.pruned {
		t.Fatal("prune was not executed")
	}
	want := time.Now().UTC().Add(-24 * time.Hour)
	if store.prunedBefore.After(want.Add(time.Minute)) {
		t.Fatalf("pruned before %v, expected retention cutoff ~%v", store.prunedBefore, want)
	}
}

func TestMetricCategory(t *testing.T) {
	cases := map[string]string{
		"system.cpu.used_percent":          "system",
		"postgres.connections.percent":     "postgres",
		"redis.memory.used_bytes":          "redis",
		"custom.metric":                    "other",
	}
	for metric, want := range cases {
		if got := metricCategory(metric); got != want {
			t.Errorf("metricCategory(%s) = %q, want %q", metric, got, want)
		}
	}
}
