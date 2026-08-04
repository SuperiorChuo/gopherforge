package monitor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	AlertStateOK      = "ok"
	AlertStatePending = "pending"
	AlertStateFiring  = "firing"
	AlertStateError   = "error"

	AlertEventFiring   = "firing"
	AlertEventResolved = "resolved"

	AlertNotifyPending = "pending"
	AlertNotifySent    = "sent"
	AlertNotifySkipped = "skipped"
	AlertNotifyFailed  = "failed"

	maxAlertDurationSeconds = 7 * 24 * 60 * 60
	maxAlertStoredText      = 1000
)

var (
	ErrAlertStoreUnavailable    = errors.New("alert store unavailable")
	ErrAlertRuleDisabled        = errors.New("alert rule is disabled")
	ErrInvalidAlertRuleName     = errors.New("invalid alert rule name")
	ErrInvalidAlertMetric       = errors.New("invalid alert metric")
	ErrInvalidAlertOperator     = errors.New("invalid alert operator")
	ErrInvalidAlertThreshold    = errors.New("invalid alert threshold")
	ErrInvalidAlertDuration     = errors.New("invalid alert duration")
	ErrInvalidAlertSeverity     = errors.New("invalid alert severity")
	ErrInvalidAlertState        = errors.New("invalid alert state")
	ErrInvalidAlertEventStatus  = errors.New("invalid alert event status")
	ErrInvalidAlertNotifyStatus = errors.New("invalid alert notification status")
	ErrAlertMetricUnavailable   = errors.New("alert metric unavailable")
)

type AlertRuleInput struct {
	Name            string
	Metric          string
	Operator        string
	Threshold       float64
	DurationSeconds int64
	Severity        string
	Enabled         bool
	NotifyOnResolve bool
	NotifyChannels  model.NotifyChannelList
	SilenceUntil    *time.Time
}

type AlertStore interface {
	GetRuleByIDContext(ctx context.Context, id uint) (*model.MonitorAlertRule, error)
	ListRulesContext(ctx context.Context, req pagination.PageRequest, filter monitordao.AlertRuleFilter) ([]model.MonitorAlertRule, int64, error)
	ListEnabledRuleIDsContext(ctx context.Context) ([]uint, error)
	GetRuleSummaryContext(ctx context.Context) (model.MonitorAlertSummary, error)
	CreateRuleContext(ctx context.Context, rule *model.MonitorAlertRule) error
	UpdateRuleContext(ctx context.Context, id uint, update monitordao.AlertRuleUpdate) (*model.MonitorAlertRule, error)
	DeleteRuleContext(ctx context.Context, id uint) error
	ApplyTransitionContext(ctx context.Context, id uint, transition monitordao.AlertTransition) (*model.MonitorAlertRule, *model.MonitorAlertEvent, error)
	RecordEvaluationErrorContext(ctx context.Context, id uint, expectedMetric string, evaluatedAt time.Time, message string) (*model.MonitorAlertRule, error)
	ListEventsContext(ctx context.Context, req pagination.PageRequest, filter monitordao.AlertEventFilter) ([]model.MonitorAlertEvent, int64, error)
	UpdateEventNotificationContext(ctx context.Context, id uint64, status, notifyError string, notifiedAt time.Time) error
}

type AlertNotification struct {
	Status     string
	Error      string
	NotifiedAt time.Time
}

type AlertNotifier interface {
	// NotifyContext delivers an alert event. rule carries the notify_channel
	// selection; event is what fired/resolved. Returns a per-call result that
	// is persisted back onto the event.
	NotifyContext(ctx context.Context, rule *model.MonitorAlertRule, event *model.MonitorAlertEvent) AlertNotification
}

type AlertService struct {
	store     AlertStore
	collector AlertMetricCollector
	notifier  AlertNotifier
	now       func() time.Time
}

func NewAlertService(db *gorm.DB, redisClient redis.UniversalClient) *AlertService {
	var store AlertStore
	if db != nil {
		store = monitordao.NewAlertDAO(db)
	}
	return NewAlertServiceWithDependencies(
		store,
		NewDefaultAlertMetricCollector(db, redisClient),
		NewMultiChannelNotifier(),
	)
}

func NewAlertServiceWithDependencies(store AlertStore, collector AlertMetricCollector, notifier AlertNotifier) *AlertService {
	return &AlertService{store: store, collector: collector, notifier: notifier, now: time.Now}
}

func (s *AlertService) ListRulesContext(ctx context.Context, req pagination.PageRequest, filter monitordao.AlertRuleFilter) ([]model.MonitorAlertRule, int64, error) {
	if err := s.ready(); err != nil {
		return nil, 0, err
	}
	return s.store.ListRulesContext(ctx, req, filter)
}

func (s *AlertService) ListEventsContext(ctx context.Context, req pagination.PageRequest, filter monitordao.AlertEventFilter) ([]model.MonitorAlertEvent, int64, error) {
	if err := s.ready(); err != nil {
		return nil, 0, err
	}
	return s.store.ListEventsContext(ctx, req, filter)
}

func (s *AlertService) GetRuleSummaryContext(ctx context.Context) (model.MonitorAlertSummary, error) {
	if err := s.ready(); err != nil {
		return model.MonitorAlertSummary{}, err
	}
	summary, err := s.store.GetRuleSummaryContext(ctx)
	if err != nil {
		return model.MonitorAlertSummary{}, err
	}
	summary.CheckedAt = s.now().UTC()
	return summary, nil
}

func (s *AlertService) GetRuleContext(ctx context.Context, id uint) (*model.MonitorAlertRule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.store.GetRuleByIDContext(ctx, id)
}

func (s *AlertService) CreateRuleContext(ctx context.Context, input AlertRuleInput) (*model.MonitorAlertRule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	input = normalizeAlertRuleInput(input)
	if err := validateAlertRuleInput(input); err != nil {
		return nil, err
	}
	rule := &model.MonitorAlertRule{
		Name:            input.Name,
		Metric:          input.Metric,
		Operator:        input.Operator,
		Threshold:       input.Threshold,
		DurationSeconds: input.DurationSeconds,
		Severity:        input.Severity,
		Enabled:         input.Enabled,
		NotifyOnResolve: input.NotifyOnResolve,
		NotifyChannels:  append(model.NotifyChannelList(nil), input.NotifyChannels...),
		SilenceUntil:    input.SilenceUntil,
		State:           AlertStateOK,
	}
	if err := s.store.CreateRuleContext(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *AlertService) UpdateRuleContext(ctx context.Context, id uint, input AlertRuleInput) (*model.MonitorAlertRule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	input = normalizeAlertRuleInput(input)
	if err := validateAlertRuleInput(input); err != nil {
		return nil, err
	}
	return s.store.UpdateRuleContext(ctx, id, func(rule *model.MonitorAlertRule) error {
		applyAlertRuleInput(rule, input)
		return nil
	})
}

func (s *AlertService) DeleteRuleContext(ctx context.Context, id uint) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.store.DeleteRuleContext(ctx, id)
}

func (s *AlertService) EvaluateRuleContext(ctx context.Context, id uint) (*model.MonitorAlertRule, *model.MonitorAlertEvent, error) {
	if err := s.ready(); err != nil {
		return nil, nil, err
	}
	rule, err := s.store.GetRuleByIDContext(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !rule.Enabled {
		return nil, nil, ErrAlertRuleDisabled
	}
	evaluatedAt := s.now().UTC()
	// During a silence window (maintenance) the rule is not evaluated, so no
	// transitions happen and no notification is sent until it ends.
	if rule.SilenceUntil != nil && evaluatedAt.Before(*rule.SilenceUntil) {
		return rule, nil, nil
	}
	value, err := s.collector.CollectContext(ctx, rule.Metric)
	if err != nil {
		message := truncateAlertText(err.Error())
		_, persistErr := s.store.RecordEvaluationErrorContext(ctx, id, rule.Metric, evaluatedAt, message)
		if errors.Is(persistErr, monitordao.ErrAlertRuleChanged) {
			current, currentErr := s.store.GetRuleByIDContext(ctx, id)
			return current, nil, currentErr
		}
		if persistErr != nil {
			return nil, nil, errors.Join(fmt.Errorf("%w: %v", ErrAlertMetricUnavailable, err), persistErr)
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrAlertMetricUnavailable, err)
	}
	updated, event, err := s.store.ApplyTransitionContext(ctx, id, func(locked *model.MonitorAlertRule) (*model.MonitorAlertEvent, error) {
		if !locked.Enabled {
			return nil, ErrAlertRuleDisabled
		}
		if locked.Metric != rule.Metric {
			return nil, monitordao.ErrAlertRuleChanged
		}
		return transitionAlertRule(locked, value, evaluatedAt)
	})
	if err != nil {
		if errors.Is(err, monitordao.ErrAlertRuleChanged) {
			current, currentErr := s.store.GetRuleByIDContext(ctx, id)
			return current, nil, currentErr
		}
		return nil, nil, err
	}
	if event == nil {
		return updated, nil, nil
	}
	notification := AlertNotification{Status: AlertNotifySkipped, NotifiedAt: s.now().UTC()}
	if event.Status != AlertEventResolved || updated.NotifyOnResolve {
		if s.notifier != nil {
			notification = s.notifier.NotifyContext(ctx, updated, event)
		}
	}
	if notification.NotifiedAt.IsZero() {
		notification.NotifiedAt = s.now().UTC()
	}
	notification.Status = normalizeNotificationStatus(notification.Status)
	notification.Error = truncateAlertText(notification.Error)
	if err := s.store.UpdateEventNotificationContext(ctx, event.ID, notification.Status, notification.Error, notification.NotifiedAt); err != nil {
		return updated, event, err
	}
	event.NotifyStatus = notification.Status
	event.NotifyError = notification.Error
	event.NotifiedAt = &notification.NotifiedAt
	return updated, event, nil
}

func (s *AlertService) EvaluateAllContext(ctx context.Context) (int, int, error) {
	if err := s.ready(); err != nil {
		return 0, 0, err
	}
	ids, err := s.store.ListEnabledRuleIDsContext(ctx)
	if err != nil {
		return 0, 0, err
	}
	succeeded := 0
	failed := 0
	errs := make([]error, 0)
	for _, id := range ids {
		if _, _, err := s.EvaluateRuleContext(ctx, id); err != nil {
			failed++
			errs = append(errs, fmt.Errorf("rule %d: %w", id, err))
			continue
		}
		succeeded++
	}
	return succeeded, failed, errors.Join(errs...)
}

func (s *AlertService) ready() error {
	if s == nil || s.store == nil || s.collector == nil {
		return ErrAlertStoreUnavailable
	}
	return nil
}

func normalizeAlertRuleInput(input AlertRuleInput) AlertRuleInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Metric = strings.TrimSpace(input.Metric)
	input.Operator = strings.ToLower(strings.TrimSpace(input.Operator))
	input.Severity = strings.ToLower(strings.TrimSpace(input.Severity))
	if input.Severity == "" {
		input.Severity = "warning"
	}
	return input
}

func validateAlertRuleInput(input AlertRuleInput) error {
	if input.Name == "" || len([]rune(input.Name)) > 100 {
		return ErrInvalidAlertRuleName
	}
	if !isKnownAlertMetric(input.Metric) {
		return ErrInvalidAlertMetric
	}
	if !isAlertOperator(input.Operator) {
		return ErrInvalidAlertOperator
	}
	if math.IsNaN(input.Threshold) || math.IsInf(input.Threshold, 0) {
		return ErrInvalidAlertThreshold
	}
	unit := alertMetricUnit(input.Metric)
	if input.Threshold < 0 || (unit == "percent" && input.Threshold > 100) {
		return ErrInvalidAlertThreshold
	}
	if (unit == "bytes" || unit == "count") && math.Trunc(input.Threshold) != input.Threshold {
		return ErrInvalidAlertThreshold
	}
	if input.DurationSeconds < 0 || input.DurationSeconds > maxAlertDurationSeconds {
		return ErrInvalidAlertDuration
	}
	if !isAlertSeverity(input.Severity) {
		return ErrInvalidAlertSeverity
	}
	return nil
}

func applyAlertRuleInput(rule *model.MonitorAlertRule, input AlertRuleInput) {
	conditionChanged := rule.Metric != input.Metric || rule.Operator != input.Operator ||
		rule.Threshold != input.Threshold || rule.DurationSeconds != input.DurationSeconds
	rule.Name = input.Name
	rule.Metric = input.Metric
	rule.Operator = input.Operator
	rule.Threshold = input.Threshold
	rule.DurationSeconds = input.DurationSeconds
	rule.Severity = input.Severity
	rule.Enabled = input.Enabled
	rule.NotifyOnResolve = input.NotifyOnResolve
	rule.NotifyChannels = append(model.NotifyChannelList(nil), input.NotifyChannels...)
	rule.SilenceUntil = input.SilenceUntil
	if conditionChanged || !input.Enabled {
		resetAlertRuleState(rule)
	}
}

func ValidateAlertState(value string) error {
	if value == "" || value == AlertStateOK || value == AlertStatePending || value == AlertStateFiring || value == AlertStateError {
		return nil
	}
	return ErrInvalidAlertState
}

func ValidateAlertEventStatus(value string) error {
	if value == "" || value == AlertEventFiring || value == AlertEventResolved {
		return nil
	}
	return ErrInvalidAlertEventStatus
}

func ValidateAlertSeverity(value string) error {
	if value == "" || isAlertSeverity(value) {
		return nil
	}
	return ErrInvalidAlertSeverity
}

func ValidateAlertNotifyStatus(value string) error {
	if value == "" || value == AlertNotifyPending || value == AlertNotifySent || value == AlertNotifySkipped || value == AlertNotifyFailed {
		return nil
	}
	return ErrInvalidAlertNotifyStatus
}

func transitionAlertRule(rule *model.MonitorAlertRule, value float64, evaluatedAt time.Time) (*model.MonitorAlertEvent, error) {
	if rule == nil || !rule.Enabled {
		return nil, ErrAlertRuleDisabled
	}
	rule.LastValue = float64Pointer(value)
	rule.LastEvaluatedAt = timePointer(evaluatedAt)
	rule.LastError = ""
	if alertConditionMatches(rule.Operator, value, rule.Threshold) {
		if rule.FiringSince != nil {
			rule.State = AlertStateFiring
			rule.PendingSince = nil
			return nil, nil
		}
		if rule.PendingSince == nil {
			rule.PendingSince = timePointer(evaluatedAt)
		}
		if evaluatedAt.Sub(*rule.PendingSince) < time.Duration(rule.DurationSeconds)*time.Second {
			rule.State = AlertStatePending
			return nil, nil
		}
		rule.State = AlertStateFiring
		rule.PendingSince = nil
		rule.FiringSince = timePointer(evaluatedAt)
		return newAlertEvent(rule, AlertEventFiring, value, evaluatedAt), nil
	}
	wasFiring := rule.FiringSince != nil
	resetAlertRuleState(rule)
	rule.LastValue = float64Pointer(value)
	rule.LastEvaluatedAt = timePointer(evaluatedAt)
	if wasFiring {
		return newAlertEvent(rule, AlertEventResolved, value, evaluatedAt), nil
	}
	return nil, nil
}

func newAlertEvent(rule *model.MonitorAlertRule, status string, value float64, createdAt time.Time) *model.MonitorAlertEvent {
	return &model.MonitorAlertEvent{
		RuleName:     rule.Name,
		Metric:       rule.Metric,
		Severity:     rule.Severity,
		Status:       status,
		Value:        value,
		Threshold:    rule.Threshold,
		Message:      truncateAlertText(alertEventMessage(rule, status, value)),
		NotifyStatus: AlertNotifyPending,
		CreatedAt:    createdAt,
	}
}

func alertEventMessage(rule *model.MonitorAlertRule, status string, value float64) string {
	if status == AlertEventResolved {
		return fmt.Sprintf("Alert %q resolved: %s is %.4f, threshold %s %.4f", rule.Name, rule.Metric, value, rule.Operator, rule.Threshold)
	}
	return fmt.Sprintf("Alert %q firing: %s is %.4f, threshold %s %.4f", rule.Name, rule.Metric, value, rule.Operator, rule.Threshold)
}

func resetAlertRuleState(rule *model.MonitorAlertRule) {
	rule.State = AlertStateOK
	rule.PendingSince = nil
	rule.FiringSince = nil
	rule.LastError = ""
}

func alertConditionMatches(operator string, value, threshold float64) bool {
	switch operator {
	case "gt":
		return value > threshold
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	default:
		return false
	}
}

func isAlertOperator(value string) bool {
	return value == "gt" || value == "gte" || value == "lt" || value == "lte"
}

func isAlertSeverity(value string) bool {
	return value == "info" || value == "warning" || value == "critical"
}

func normalizeNotificationStatus(value string) string {
	switch value {
	case AlertNotifySent, AlertNotifySkipped, AlertNotifyFailed:
		return value
	default:
		return AlertNotifyFailed
	}
}

func truncateAlertText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxAlertStoredText {
		return string(runes)
	}
	return string(runes[:maxAlertStoredText])
}

func float64Pointer(value float64) *float64 {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}
