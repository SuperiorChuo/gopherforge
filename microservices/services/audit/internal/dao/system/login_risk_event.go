package system

import (
	"context"
	"time"

	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"gorm.io/gorm"
)

// LoginRiskEventDAO persists abnormal-login events (new IP / new device).
type LoginRiskEventDAO struct {
	db *gorm.DB
}

func NewLoginRiskEventDAO(db *gorm.DB) *LoginRiskEventDAO {
	return &LoginRiskEventDAO{db: db}
}

func (d *LoginRiskEventDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *LoginRiskEventDAO) CreateContext(ctx context.Context, e *localmodel.LoginRiskEvent) error {
	return d.dbWithContext(ctx).Create(e).Error
}

// MarkNotifiedContext records that the alert was sent for an event.
func (d *LoginRiskEventDAO) MarkNotifiedContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Model(&localmodel.LoginRiskEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{"alerted": true, "notified_at": time.Now()}).Error
}

// MarkProcessedContext marks an event handled by an admin.
func (d *LoginRiskEventDAO) MarkProcessedContext(ctx context.Context, id uint, by uint) error {
	return d.dbWithContext(ctx).Model(&localmodel.LoginRiskEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{"processed": true, "processed_by": by, "processed_at": time.Now()}).Error
}

// DeleteBeforeContext removes risk events older than the cutoff (retention).
func (d *LoginRiskEventDAO) DeleteBeforeContext(ctx context.Context, before time.Time) (int64, error) {
	res := d.dbWithContext(ctx).
		Where("created_at < ?", before).
		Delete(&localmodel.LoginRiskEvent{})
	return res.RowsAffected, res.Error
}

type LoginRiskEventFilter struct {
	UserID    uint
	Username  string
	IP        string
	Reason    string
	Processed *bool
	// DataScope 与登录日志页一致：部门数据范围的普通管理员只能看本部门用户的异常登录。
	DataScope authz.UserDataScope
}

func (d *LoginRiskEventDAO) ListContext(ctx context.Context, req pagination.PageRequest, filter LoginRiskEventFilter) ([]localmodel.LoginRiskEvent, int64, error) {
	q := d.dbWithContext(authz.EnableDataScope(ctx, filter.DataScope)).Model(&localmodel.LoginRiskEvent{})
	if filter.UserID > 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.Username != "" {
		q = q.Where("username ILIKE ?", "%"+filter.Username+"%")
	}
	if filter.IP != "" {
		q = q.Where("ip = ?", filter.IP)
	}
	if filter.Reason != "" {
		q = q.Where("reason = ?", filter.Reason)
	}
	if filter.Processed != nil {
		q = q.Where("processed = ?", *filter.Processed)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []localmodel.LoginRiskEvent
	if err := q.
		Order("created_at DESC, id DESC").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}
