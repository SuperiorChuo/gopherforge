package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	"github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/mailer"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
)

func TestTransitionAlertRuleEmitsOneFiringAndOneResolvedEvent(t *testing.T) {
	startedAt := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)
	rule := &model.MonitorAlertRule{
		ID:              7,
		Name:            "High CPU",
		Metric:          "system.cpu.used_percent",
		Operator:        "gte",
		Threshold:       80,
		DurationSeconds: 60,
		Severity:        "critical",
		Enabled:         true,
		State:           AlertStateOK,
	}

	event, err := transitionAlertRule(rule, 85, startedAt)
	if err != nil {
		t.Fatalf("first transition error = %v", err)
	}
	if event != nil || rule.State != AlertStatePending || rule.PendingSince == nil {
		t.Fatalf("first transition = state %q event %#v pending %#v, want pending without event", rule.State, event, rule.PendingSince)
	}

	event, err = transitionAlertRule(rule, 86, startedAt.Add(59*time.Second))
	if err != nil {
		t.Fatalf("pre-boundary transition error = %v", err)
	}
	if event != nil || rule.State != AlertStatePending {
		t.Fatalf("pre-boundary transition = state %q event %#v, want pending", rule.State, event)
	}

	event, err = transitionAlertRule(rule, 87, startedAt.Add(60*time.Second))
	if err != nil {
		t.Fatalf("firing transition error = %v", err)
	}
	if event == nil || event.Status != AlertEventFiring || rule.State != AlertStateFiring || rule.FiringSince == nil {
		t.Fatalf("firing transition = state %q event %#v firing %#v", rule.State, event, rule.FiringSince)
	}

	event, err = transitionAlertRule(rule, 90, startedAt.Add(90*time.Second))
	if err != nil {
		t.Fatalf("steady firing transition error = %v", err)
	}
	if event != nil || rule.State != AlertStateFiring {
		t.Fatalf("steady firing transition = state %q event %#v, want no duplicate", rule.State, event)
	}

	event, err = transitionAlertRule(rule, 70, startedAt.Add(120*time.Second))
	if err != nil {
		t.Fatalf("resolved transition error = %v", err)
	}
	if event == nil || event.Status != AlertEventResolved || rule.State != AlertStateOK || rule.FiringSince != nil {
		t.Fatalf("resolved transition = state %q event %#v firing %#v", rule.State, event, rule.FiringSince)
	}

	event, err = transitionAlertRule(rule, 60, startedAt.Add(150*time.Second))
	if err != nil {
		t.Fatalf("steady ok transition error = %v", err)
	}
	if event != nil {
		t.Fatalf("steady ok emitted duplicate resolved event: %#v", event)
	}
}

func TestTransitionAlertRulePreservesIncidentAcrossCollectionError(t *testing.T) {
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	firingSince := now.Add(-time.Minute)
	rule := &model.MonitorAlertRule{
		Name:        "Redis clients",
		Metric:      "redis.clients.connected",
		Operator:    "gt",
		Threshold:   10,
		Severity:    "warning",
		Enabled:     true,
		State:       AlertStateError,
		FiringSince: &firingSince,
		LastError:   "redis unavailable",
	}

	event, err := transitionAlertRule(rule, 11, now)
	if err != nil {
		t.Fatalf("recovered firing transition error = %v", err)
	}
	if event != nil || rule.State != AlertStateFiring || rule.FiringSince == nil || rule.LastError != "" {
		t.Fatalf("recovered firing transition = state %q event %#v firing %#v error %q", rule.State, event, rule.FiringSince, rule.LastError)
	}

	event, err = transitionAlertRule(rule, 8, now.Add(time.Second))
	if err != nil {
		t.Fatalf("recovered resolved transition error = %v", err)
	}
	if event == nil || event.Status != AlertEventResolved {
		t.Fatalf("recovered resolved event = %#v, want resolved", event)
	}
}

func TestAlertRuleValidationRejectsInvalidBoundaryValues(t *testing.T) {
	valid := AlertRuleInput{
		Name:            "Disk usage",
		Metric:          "system.disk.used_percent",
		Operator:        "gt",
		Threshold:       90,
		DurationSeconds: 30,
		Severity:        "warning",
		Enabled:         true,
		NotifyOnResolve: true,
	}
	if err := validateAlertRuleInput(valid); err != nil {
		t.Fatalf("valid input error = %v", err)
	}

	tests := []struct {
		name string
		edit func(*AlertRuleInput)
		want error
	}{
		{name: "metric", edit: func(input *AlertRuleInput) { input.Metric = "unknown" }, want: ErrInvalidAlertMetric},
		{name: "operator", edit: func(input *AlertRuleInput) { input.Operator = "eq" }, want: ErrInvalidAlertOperator},
		{name: "percent threshold", edit: func(input *AlertRuleInput) { input.Threshold = 101 }, want: ErrInvalidAlertThreshold},
		{name: "duration", edit: func(input *AlertRuleInput) { input.DurationSeconds = maxAlertDurationSeconds + 1 }, want: ErrInvalidAlertDuration},
		{name: "severity", edit: func(input *AlertRuleInput) { input.Severity = "emergency" }, want: ErrInvalidAlertSeverity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.edit(&input)
			if err := validateAlertRuleInput(input); !errors.Is(err, tt.want) {
				t.Fatalf("validation error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestApplyAlertRuleInputPreservesOrResetsDurableState(t *testing.T) {
	pendingSince := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	firingSince := pendingSince.Add(time.Minute)
	rule := &model.MonitorAlertRule{
		Name:            "High CPU",
		Metric:          "system.cpu.used_percent",
		Operator:        "gte",
		Threshold:       80,
		DurationSeconds: 60,
		Severity:        "warning",
		Enabled:         true,
		NotifyOnResolve: true,
		State:           AlertStateFiring,
		PendingSince:    &pendingSince,
		FiringSince:     &firingSince,
		LastError:       "collector unavailable",
	}

	metadataOnly := AlertRuleInput{
		Name:            "High CPU renamed",
		Metric:          rule.Metric,
		Operator:        rule.Operator,
		Threshold:       rule.Threshold,
		DurationSeconds: rule.DurationSeconds,
		Severity:        "critical",
		Enabled:         true,
		NotifyOnResolve: false,
	}
	applyAlertRuleInput(rule, metadataOnly)
	if rule.State != AlertStateFiring || rule.FiringSince == nil {
		t.Fatalf("metadata update reset durable incident: state=%q firing_since=%v", rule.State, rule.FiringSince)
	}

	conditionChange := metadataOnly
	conditionChange.Threshold = 90
	applyAlertRuleInput(rule, conditionChange)
	if rule.State != AlertStateOK || rule.PendingSince != nil || rule.FiringSince != nil || rule.LastError != "" {
		t.Fatalf("condition update did not reset state: %#v", rule)
	}
}

func TestNewAlertServiceWithoutDatabaseFailsClosed(t *testing.T) {
	service := NewAlertService(nil, nil)
	_, _, err := service.ListRulesContext(context.Background(), pagination.PageRequest{Page: 1, PageSize: 10}, monitordao.AlertRuleFilter{})
	if !errors.Is(err, ErrAlertStoreUnavailable) {
		t.Fatalf("ListRulesContext error = %v, want %v", err, ErrAlertStoreUnavailable)
	}
}

func TestAlertEmailNotifierRecordsSentSkippedAndFailed(t *testing.T) {
	event := &model.MonitorAlertEvent{
		ID:        12,
		RuleName:  "High memory",
		Metric:    "system.memory.used_percent",
		Severity:  "critical",
		Status:    AlertEventFiring,
		Value:     95,
		Threshold: 90,
		Message:   "memory threshold exceeded",
		CreatedAt: time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	}

	t.Run("sent uses alert recipient group", func(t *testing.T) {
		sender := &alertEmailSenderStub{}
		notifier := NewAlertEmailNotifier(sender, alertEmailReaderStub{policy: runtimeconfig.EmailNotification{
			Enabled:        true,
			Sender:         "alerts@example.com",
			AlertReceivers: []string{"fallback@example.com"},
			RecipientGroups: map[string][]string{
				"alert": {"ops@example.com"},
			},
		}})
		result := notifier.NotifyContext(context.Background(), nil, event)
		if result.Status != AlertNotifySent || len(sender.messages) != 1 {
			t.Fatalf("notification = %#v messages = %d, want sent once", result, len(sender.messages))
		}
		if got := sender.messages[0].To; len(got) != 1 || got[0] != "ops@example.com" {
			t.Fatalf("recipients = %#v, want alert group", got)
		}
	})

	t.Run("skipped when channel disabled", func(t *testing.T) {
		sender := &alertEmailSenderStub{}
		notifier := NewAlertEmailNotifier(sender, alertEmailReaderStub{policy: runtimeconfig.EmailNotification{
			Enabled:        false,
			AlertReceivers: []string{"ops@example.com"},
		}})
		result := notifier.NotifyContext(context.Background(), nil, event)
		if result.Status != AlertNotifySkipped || len(sender.messages) != 0 {
			t.Fatalf("notification = %#v messages = %d, want skipped", result, len(sender.messages))
		}
	})

	t.Run("failed stores bounded sender error", func(t *testing.T) {
		sender := &alertEmailSenderStub{err: errors.New("smtp unavailable")}
		notifier := NewAlertEmailNotifier(sender, alertEmailReaderStub{policy: runtimeconfig.EmailNotification{
			Enabled:        true,
			Sender:         "alerts@example.com",
			AlertReceivers: []string{"ops@example.com"},
		}})
		result := notifier.NotifyContext(context.Background(), nil, event)
		if result.Status != AlertNotifyFailed || result.Error != "smtp unavailable" {
			t.Fatalf("notification = %#v, want failed sender error", result)
		}
	})
}

type alertEmailSenderStub struct {
	messages []mailer.Message
	err      error
}

func (s *alertEmailSenderStub) Send(_ context.Context, message mailer.Message) error {
	s.messages = append(s.messages, message)
	return s.err
}

type alertEmailReaderStub struct {
	policy runtimeconfig.EmailNotification
}

func (s alertEmailReaderStub) EmailNotification(context.Context) runtimeconfig.EmailNotification {
	return s.policy
}

type stubAlertStore struct {
	rule *model.MonitorAlertRule
}

func (s *stubAlertStore) GetRuleByIDContext(context.Context, uint) (*model.MonitorAlertRule, error) {
	return s.rule, nil
}
func (s *stubAlertStore) ListRulesContext(context.Context, pagination.PageRequest, monitordao.AlertRuleFilter) ([]model.MonitorAlertRule, int64, error) {
	return nil, 0, nil
}
func (s *stubAlertStore) ListEnabledRuleIDsContext(context.Context) ([]uint, error) { return nil, nil }
func (s *stubAlertStore) GetRuleSummaryContext(context.Context) (model.MonitorAlertSummary, error) {
	return model.MonitorAlertSummary{}, nil
}
func (s *stubAlertStore) CreateRuleContext(context.Context, *model.MonitorAlertRule) error { return nil }
func (s *stubAlertStore) UpdateRuleContext(context.Context, uint, monitordao.AlertRuleUpdate) (*model.MonitorAlertRule, error) {
	return nil, nil
}
func (s *stubAlertStore) DeleteRuleContext(context.Context, uint) error { return nil }
func (s *stubAlertStore) ApplyTransitionContext(context.Context, uint, monitordao.AlertTransition) (*model.MonitorAlertRule, *model.MonitorAlertEvent, error) {
	return nil, nil, nil
}
func (s *stubAlertStore) RecordEvaluationErrorContext(context.Context, uint, string, time.Time, string) (*model.MonitorAlertRule, error) {
	return nil, nil
}
func (s *stubAlertStore) ListEventsContext(context.Context, pagination.PageRequest, monitordao.AlertEventFilter) ([]model.MonitorAlertEvent, int64, error) {
	return nil, 0, nil
}
func (s *stubAlertStore) UpdateEventNotificationContext(context.Context, uint64, string, string, time.Time) error {
	return nil
}

func TestEvaluateRuleContextSkipsSilencedRule(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC()
	store := &stubAlertStore{rule: &model.MonitorAlertRule{
		ID:           1,
		Enabled:      true,
		Metric:       "system.cpu.used_percent",
		State:        AlertStateOK,
		SilenceUntil: &future,
	}}
	svc := NewAlertServiceWithDependencies(store, &fakeAlertMetricCollector{}, nil)
	svc.now = func() time.Time { return time.Now().UTC() }

	rule, event, err := svc.EvaluateRuleContext(context.Background(), 1)
	if err != nil {
		t.Fatalf("EvaluateRuleContext error = %v, want nil", err)
	}
	if event != nil {
		t.Fatalf("event = %#v, want nil for a silenced rule", event)
	}
	if rule == nil || rule.ID != 1 {
		t.Fatalf("rule = %#v, want the rule returned untouched", rule)
	}
}
