package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
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

type AlertTransition func(rule *model.MonitorAlertRule) (*model.MonitorAlertEvent, error)

type AlertRuleUpdate func(rule *model.MonitorAlertRule) error

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

func (d *AlertDAO) GetRuleByIDContext(ctx context.Context, id uint) (*model.MonitorAlertRule, error) {
	var rule model.MonitorAlertRule
	err := d.dbWithContext(ctx).First(&rule, id).Error
	return &rule, err
}

func (d *AlertDAO) ListRulesContext(ctx context.Context, req pagination.PageRequest, filter AlertRuleFilter) ([]model.MonitorAlertRule, int64, error) {
	var rules []model.MonitorAlertRule
	var total int64
	query := d.dbWithContext(ctx).Model(&model.MonitorAlertRule{})
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
	err := d.dbWithContext(ctx).Model(&model.MonitorAlertRule{}).
		Where("enabled = ?", true).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

func (d *AlertDAO) GetRuleSummaryContext(ctx context.Context) (model.MonitorAlertSummary, error) {
	var summary model.MonitorAlertSummary
	err := d.dbWithContext(ctx).Model(&model.MonitorAlertRule{}).
		Select(`
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN enabled THEN 1 ELSE 0 END), 0) AS enabled,
			COALESCE(SUM(CASE WHEN firing_since IS NOT NULL THEN 1 ELSE 0 END), 0) AS firing,
			COALESCE(SUM(CASE WHEN state = 'pending' THEN 1 ELSE 0 END), 0) AS pending,
			COALESCE(SUM(CASE WHEN state = 'error' THEN 1 ELSE 0 END), 0) AS error
		`).Scan(&summary).Error
	return summary, err
}

func (d *AlertDAO) CreateRuleContext(ctx context.Context, rule *model.MonitorAlertRule) error {
	return d.dbWithContext(ctx).Create(rule).Error
}

func (d *AlertDAO) UpdateRuleContext(ctx context.Context, id uint, update AlertRuleUpdate) (*model.MonitorAlertRule, error) {
	if id == 0 || update == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var updated model.MonitorAlertRule
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule model.MonitorAlertRule
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
	result := d.dbWithContext(ctx).Delete(&model.MonitorAlertRule{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *AlertDAO) ApplyTransitionContext(ctx context.Context, id uint, transition AlertTransition) (*model.MonitorAlertRule, *model.MonitorAlertEvent, error) {
	var updated model.MonitorAlertRule
	var created *model.MonitorAlertEvent
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule model.MonitorAlertRule
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

func (d *AlertDAO) RecordEvaluationErrorContext(ctx context.Context, id uint, expectedMetric string, evaluatedAt time.Time, message string) (*model.MonitorAlertRule, error) {
	var updated model.MonitorAlertRule
	err := d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule model.MonitorAlertRule
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

func markAlertEvaluationError(rule *model.MonitorAlertRule, expectedMetric string, evaluatedAt time.Time, message string) error {
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

func (d *AlertDAO) ListEventsContext(ctx context.Context, req pagination.PageRequest, filter AlertEventFilter) ([]model.MonitorAlertEvent, int64, error) {
	var events []model.MonitorAlertEvent
	var total int64
	query := d.dbWithContext(ctx).Model(&model.MonitorAlertEvent{})
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
	result := d.dbWithContext(ctx).Model(&model.MonitorAlertEvent{}).
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
