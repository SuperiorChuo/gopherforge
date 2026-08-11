package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AlertRuleFilter struct {
	Name    string
	Metric  string
	State   string
	Enabled *bool
}

type AlertEventFilter struct {
	RuleID       uint
	RuleName     string
	Status       string
	Severity     string
	NotifyStatus string
}

type AlertTransition func(rule *localmodel.MonitorAlertRule) (*localmodel.MonitorAlertEvent, error)

type AlertRuleUpdate func(rule *localmodel.MonitorAlertRule) error

var ErrAlertRuleChanged = errors.New("alert rule changed during evaluation")

type AlertDAO struct {
	db *gorm.DB
}

func NewAlertDAO(db *gorm.DB) *AlertDAO {
	return &AlertDAO{db: db}
}

func (d *AlertDAO) Ready() bool {
	return d != nil && d.db != nil
}

func (d *AlertDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *AlertDAO) GetRuleByIDContext(ctx context.Context, id uint) (*localmodel.MonitorAlertRule, error) {
	var rule localmodel.MonitorAlertRule
	err := d.dbWithContext(ctx).First(&rule, id).Error
	return &rule, err
}

func (d *AlertDAO) ListRulesContext(ctx context.Context, req pagination.PageRequest, filter AlertRuleFilter) ([]localmodel.MonitorAlertRule, int64, error) {
	var rules []localmodel.MonitorAlertRule
	var total int64
	query := d.dbWithContext(ctx).Model(&localmodel.MonitorAlertRule{})
	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}
	if filter.Metric != "" {
		query = query.Where("metric = ?", filter.Metric)
	}
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Scopes(pagination.Paginate(req)).Order("created_at DESC, id DESC").Find(&rules).Error
	return rules, total, err
}

func (d *AlertDAO) ListEnabledRuleIDsContext(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := d.dbWithContext(ctx).Model(&localmodel.MonitorAlertRule{}).
		Where("enabled = ?", true).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

func (d *AlertDAO) GetRuleSummaryContext(ctx context.Context) (localmodel.MonitorAlertSummary, error) {
	var summary localmodel.MonitorAlertSummary
	err := d.dbWithContext(ctx).Model(&localmodel.MonitorAlertRule{}).
		Select(`
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN enabled THEN 1 ELSE 0 END), 0) AS enabled,
			COALESCE(SUM(CASE WHEN firing_since IS NOT NULL THEN 1 ELSE 0 END), 0) AS firing,
			COALESCE(SUM(CASE WHEN state = 'pending' THEN 1 ELSE 0 END), 0) AS pending,
			COALESCE(SUM(CASE WHEN state = 'error' THEN 1 ELSE 0 END), 0) AS error
		`).Scan(&summary).Error
	return summary, err
}

func (d *AlertDAO) CreateRuleContext(ctx context.Context, rule *localmodel.MonitorAlertRule) error {
	return d.dbWithContext(ctx).Create(rule).Error
}

func (d *AlertDAO) UpdateRuleContext(ctx context.Context, id uint, update AlertRuleUpdate) (*localmodel.MonitorAlertRule, error) {
	if id == 0 || update == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var updated localmodel.MonitorAlertRule
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule localmodel.MonitorAlertRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, id).Error; err != nil {
			return err
		}
		if err := update(&rule); err != nil {
			return err
		}
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		updated = rule
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (d *AlertDAO) DeleteRuleContext(ctx context.Context, id uint) error {
	result := d.dbWithContext(ctx).Delete(&localmodel.MonitorAlertRule{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *AlertDAO) ApplyTransitionContext(ctx context.Context, id uint, transition AlertTransition) (*localmodel.MonitorAlertRule, *localmodel.MonitorAlertEvent, error) {
	var updated localmodel.MonitorAlertRule
	var created *localmodel.MonitorAlertEvent
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule localmodel.MonitorAlertRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, id).Error; err != nil {
			return err
		}
		event, err := transition(&rule)
		if err != nil {
			return err
		}
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		if event != nil {
			ruleID := rule.ID
			event.RuleID = &ruleID
			if err := tx.Create(event).Error; err != nil {
				return err
			}
			copyEvent := *event
			created = &copyEvent
		}
		updated = rule
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &updated, created, nil
}

func (d *AlertDAO) RecordEvaluationErrorContext(ctx context.Context, id uint, expectedMetric string, evaluatedAt time.Time, message string) (*localmodel.MonitorAlertRule, error) {
	var updated localmodel.MonitorAlertRule
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule localmodel.MonitorAlertRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, id).Error; err != nil {
			return err
		}
		if !rule.Enabled {
			return nil
		}
		if err := markAlertEvaluationError(&rule, expectedMetric, evaluatedAt, message); err != nil {
			return err
		}
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		updated = rule
		return nil
	})
	if err != nil {
		return nil, err
	}
	if updated.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &updated, nil
}

func markAlertEvaluationError(rule *localmodel.MonitorAlertRule, expectedMetric string, evaluatedAt time.Time, message string) error {
	if rule.Metric != expectedMetric {
		return fmt.Errorf("%w: expected metric %q, found %q", ErrAlertRuleChanged, expectedMetric, rule.Metric)
	}
	rule.State = "error"
	if rule.FiringSince == nil {
		rule.PendingSince = nil
	}
	rule.LastEvaluatedAt = &evaluatedAt
	rule.LastError = message
	return nil
}

func (d *AlertDAO) ListEventsContext(ctx context.Context, req pagination.PageRequest, filter AlertEventFilter) ([]localmodel.MonitorAlertEvent, int64, error) {
	var events []localmodel.MonitorAlertEvent
	var total int64
	query := d.dbWithContext(ctx).Model(&localmodel.MonitorAlertEvent{})
	if filter.RuleID > 0 {
		query = query.Where("rule_id = ?", filter.RuleID)
	}
	if filter.RuleName != "" {
		query = query.Where("rule_name ILIKE ?", "%"+filter.RuleName+"%")
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		query = query.Where("severity = ?", filter.Severity)
	}
	if filter.NotifyStatus != "" {
		query = query.Where("notify_status = ?", filter.NotifyStatus)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Scopes(pagination.Paginate(req)).Order("created_at DESC, id DESC").Find(&events).Error
	return events, total, err
}

func (d *AlertDAO) UpdateEventNotificationContext(ctx context.Context, id uint64, status, notifyError string, notifiedAt time.Time) error {
	result := d.dbWithContext(ctx).Model(&localmodel.MonitorAlertEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"notify_status": status,
			"notify_error":  notifyError,
			"notified_at":   notifiedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func IsAlertRuleNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
