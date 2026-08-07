package monitor

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/monitor/internal/model"
)

// The console dropdown and the executor must not drift apart: every listed
// target has to be dispatchable, and every dispatchable target has to be
// listed. Before the shared table they were a hardcoded switch and a free-text
// input with nothing tying them together.
func TestListJobTargetsMatchesDispatchTable(t *testing.T) {
	listed := ListJobTargets()
	if len(listed) != len(jobTargets) {
		t.Fatalf("listed %d targets, dispatch table has %d", len(listed), len(jobTargets))
	}

	for _, target := range listed {
		entry, ok := jobTargets[target.Target]
		if !ok {
			t.Errorf("listed target %q is not dispatchable", target.Target)
			continue
		}
		if entry.execute == nil {
			t.Errorf("target %q has no executor", target.Target)
		}
		if target.Title == "" || target.Description == "" {
			t.Errorf("target %q must carry a title and description for the console", target.Target)
		}
	}

	// Sorted by id so the dropdown order is stable across restarts (map
	// iteration order is not).
	for i := 1; i < len(listed); i++ {
		if listed[i-1].Target > listed[i].Target {
			t.Fatalf("targets not sorted: %q before %q", listed[i-1].Target, listed[i].Target)
		}
	}
}

func TestValidateInvokeTargetRejectsUnknown(t *testing.T) {
	for _, target := range ListJobTargets() {
		if err := validateInvokeTarget(target.Target); err != nil {
			t.Errorf("listed target %q rejected: %v", target.Target, err)
		}
	}
	// Surrounding whitespace is a paste artifact, not a different target.
	if err := validateInvokeTarget("  HealthCheck  "); err != nil {
		t.Errorf("padded target rejected: %v", err)
	}
	for _, bad := range []string{"", "healthcheck", "DropAllTables", "HealthCheck()"} {
		if err := validateInvokeTarget(bad); !errors.Is(err, ErrUnknownInvokeTarget) {
			t.Errorf("target %q: got %v, want ErrUnknownInvokeTarget", bad, err)
		}
	}
}

// Unknown targets must be refused at write time. Persisting one used to be
// allowed, and the failure only showed up in the execution log on the next
// trigger.
func TestCreateAndUpdateJobRejectUnknownTarget(t *testing.T) {
	dao := &fakeJobDAO{}
	service := newJobService(dao, false)
	defer service.Stop()

	err := service.CreateJobContext(context.Background(), &model.ScheduledJob{
		Name:           "bad-target",
		CronExpression: "0 0 * * * *",
		InvokeTarget:   "NotARealTarget",
	})
	if !errors.Is(err, ErrUnknownInvokeTarget) {
		t.Fatalf("create: got %v, want ErrUnknownInvokeTarget", err)
	}
	if len(dao.jobs) != 0 {
		t.Fatalf("rejected job must not be persisted, got %d rows", len(dao.jobs))
	}

	good := &model.ScheduledJob{
		Name:           "good-target",
		CronExpression: "0 0 * * * *",
		InvokeTarget:   "HealthCheck",
	}
	if err := service.CreateJobContext(context.Background(), good); err != nil {
		t.Fatalf("create with listed target: %v", err)
	}

	good.InvokeTarget = "StillNotReal"
	if err := service.UpdateJobContext(context.Background(), good); !errors.Is(err, ErrUnknownInvokeTarget) {
		t.Fatalf("update: got %v, want ErrUnknownInvokeTarget", err)
	}
}

// A target that predates the write-time check must still be reported into the
// execution log rather than silently skipped.
func TestExecuteTaskReportsUnknownTarget(t *testing.T) {
	service := newJobService(&fakeJobDAO{}, false)
	defer service.Stop()

	if _, err := service.executeTaskContext(context.Background(), "LegacyTarget"); !errors.Is(err, ErrUnknownInvokeTarget) {
		t.Fatalf("got %v, want ErrUnknownInvokeTarget", err)
	}
}
