package monitor

import (
	"errors"
	"testing"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
)

func TestMarkAlertEvaluationErrorResetsOnlyPendingDuration(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	pendingSince := evaluatedAt.Add(-time.Minute)
	pending := &localmodel.MonitorAlertRule{
		Metric:       "system.cpu.used_percent",
		State:        "pending",
		PendingSince: &pendingSince,
	}
	if err := markAlertEvaluationError(pending, pending.Metric, evaluatedAt, "collector unavailable"); err != nil {
		t.Fatalf("mark pending error: %v", err)
	}
	if pending.State != "error" || pending.PendingSince != nil || pending.FiringSince != nil {
		t.Fatalf("pending rule after collection error = %#v", pending)
	}

	firingSince := evaluatedAt.Add(-2 * time.Minute)
	firing := &localmodel.MonitorAlertRule{
		Metric:      "system.cpu.used_percent",
		State:       "firing",
		FiringSince: &firingSince,
	}
	if err := markAlertEvaluationError(firing, firing.Metric, evaluatedAt, "collector unavailable"); err != nil {
		t.Fatalf("mark firing error: %v", err)
	}
	if firing.State != "error" || firing.FiringSince == nil || !firing.FiringSince.Equal(firingSince) {
		t.Fatalf("firing rule lost incident anchor: %#v", firing)
	}
}

func TestMarkAlertEvaluationErrorRejectsStaleMetric(t *testing.T) {
	rule := &localmodel.MonitorAlertRule{Metric: "system.memory.used_percent", State: "ok"}
	err := markAlertEvaluationError(rule, "system.cpu.used_percent", time.Now(), "old collector failed")
	if !errors.Is(err, ErrAlertRuleChanged) {
		t.Fatalf("error = %v, want %v", err, ErrAlertRuleChanged)
	}
	if rule.State != "ok" || rule.LastError != "" {
		t.Fatalf("stale metric mutated current rule: %#v", rule)
	}
}
