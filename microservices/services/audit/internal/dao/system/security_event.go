package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
)

// SecurityEventDAO persists detected security events.
type SecurityEventDAO struct {
	db *gorm.DB
}

func NewSecurityEventDAO(db *gorm.DB) *SecurityEventDAO {
	return &SecurityEventDAO{db: db}
}

func (d *SecurityEventDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *SecurityEventDAO) CreateContext(ctx context.Context, e *model.SecurityEvent) error {
	return d.dbWithContext(ctx).Create(e).Error
}

// RecentRuleHitContext reports whether the same rule+actor fired within the
// window (dedupe: one notification per actor per rule per window).
func (d *SecurityEventDAO) RecentRuleHitContext(ctx context.Context, rule, actorID string, since time.Time) (bool, error) {
	var count int64
	err := d.dbWithContext(ctx).Model(&model.SecurityEvent{}).
		Where("rule = ? AND actor_id = ? AND occurred_at >= ?", rule, actorID, since).
		Count(&count).Error
	return count > 0, err
}

// MarkNotifiedContext records that a notification was sent for an event.
func (d *SecurityEventDAO) MarkNotifiedContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Model(&model.SecurityEvent{}).
		Where("id = ?", id).
		Update("notified_at", time.Now()).Error
}

type SecurityEventFilter struct {
	Rule     string
	Severity string
}

func (d *SecurityEventDAO) ListContext(ctx context.Context, req pagination.PageRequest, filter SecurityEventFilter) ([]model.SecurityEvent, int64, error) {
	q := d.dbWithContext(ctx).Model(&model.SecurityEvent{})
	if filter.Rule != "" {
		q = q.Where("rule = ?", filter.Rule)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []model.SecurityEvent
	if err := q.
		Order("occurred_at DESC, id DESC").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}
