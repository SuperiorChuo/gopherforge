package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
)

func TestDownsampleTrendPoints(t *testing.T) {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	raw := []localmodel.MonitorMetricSample{
		{Metric: "system.cpu.used_percent", Value: 10, CollectedAt: base},
		{Metric: "system.cpu.used_percent", Value: 20, CollectedAt: base.Add(30 * time.Second)},
		{Metric: "system.cpu.used_percent", Value: 40, CollectedAt: base.Add(60 * time.Second)},
	}
	points := downsampleTrendPoints(raw, time.Minute)
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	if points[0].Value != 15 {
		t.Errorf("first bucket value = %v, want 15", points[0].Value)
	}
	if points[1].Value != 40 {
		t.Errorf("second bucket value = %v, want 40", points[1].Value)
	}
	if points[0].Timestamp != base.UnixMilli() {
		t.Errorf("first bucket timestamp = %d, want %d", points[0].Timestamp, base.UnixMilli())
	}
}

func TestDownsampleTrendPointsEmpty(t *testing.T) {
	points := downsampleTrendPoints(nil, time.Minute)
	if points == nil || len(points) != 0 {
		t.Fatalf("empty input returned %#v, want empty slice", points)
	}
}

func TestQueryTrendContext(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeMetricSampleStore{
		raw: []localmodel.MonitorMetricSample{
			{Metric: "system.cpu.used_percent", Value: 10, CollectedAt: now.Add(-90 * time.Minute)},
			{Metric: "system.cpu.used_percent", Value: 20, CollectedAt: now.Add(-60 * time.Minute)},
			{Metric: "system.cpu.used_percent", Value: 30, CollectedAt: now.Add(-30 * time.Minute)},
		},
	}
	svc := NewMetricTrendService(store)
	svc.now = func() time.Time { return now }

	resp, err := svc.QueryTrendContext(context.Background(), "system.cpu.used_percent", "24h")
	if err != nil {
		t.Fatalf("QueryTrendContext error = %v", err)
	}
	if resp.Metric != "system.cpu.used_percent" || resp.Range != "24h" || resp.Unit != "percent" {
		t.Fatalf("response = %#v", resp)
	}
	if len(resp.Points) != 3 {
		t.Fatalf("points = %d, want 3 (one per 5-min bucket)", len(resp.Points))
	}
}

func TestQueryTrendContextInvalidInputs(t *testing.T) {
	now := time.Now()
	svc := NewMetricTrendService(&fakeMetricSampleStore{})
	svc.now = func() time.Time { return now }

	if _, err := svc.QueryTrendContext(context.Background(), "system.cpu.used_percent", "45m"); !errors.Is(err, ErrInvalidTrendRange) {
		t.Fatalf("bad range error = %v, want ErrInvalidTrendRange", err)
	}
	if _, err := svc.QueryTrendContext(context.Background(), "system.nope", "1h"); !errors.Is(err, ErrInvalidAlertMetric) {
		t.Fatalf("bad metric error = %v, want ErrInvalidAlertMetric", err)
	}
}
