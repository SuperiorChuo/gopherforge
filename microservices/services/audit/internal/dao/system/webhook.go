package system

import (
	"context"
	"errors"
	"time"

	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrWebhookNotFound = errors.New("webhook subscription not found")

type WebhookDAO struct{ db *gorm.DB }

func NewWebhookDAO(db *gorm.DB) *WebhookDAO { return &WebhookDAO{db: db} }

func (d *WebhookDAO) scoped(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return tenant.ApplyFilter(d.db.WithContext(ctx), ctx)
}

func (d *WebhookDAO) ListSubscriptions(ctx context.Context, page, pageSize int) ([]localmodel.WebhookSubscription, int64, error) {
	q := d.scoped(ctx).Model(&localmodel.WebhookSubscription{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []localmodel.WebhookSubscription
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

func (d *WebhookDAO) GetSubscription(ctx context.Context, id uint) (*localmodel.WebhookSubscription, error) {
	var row localmodel.WebhookSubscription
	if err := d.scoped(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (d *WebhookDAO) CreateSubscription(ctx context.Context, row *localmodel.WebhookSubscription) error {
	row.TenantID = tenant.FromContextOrDefault(ctx)
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxAuditID uint
		if err := tx.Model(&localmodel.AuditLog{}).Where("tenant_id = ?", row.TenantID).Select("COALESCE(MAX(id), 0) AS id").Scan(&maxAuditID).Error; err != nil {
			return err
		}
		row.StartAuditLogID = maxAuditID
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		cursor := localmodel.WebhookCursor{TenantID: row.TenantID, LastAuditLogID: maxAuditID, UpdatedAt: time.Now()}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cursor).Error
	})
}

func (d *WebhookDAO) UpdateSubscription(ctx context.Context, id uint, values map[string]any) error {
	result := d.scoped(ctx).Model(&localmodel.WebhookSubscription{}).Where("id = ?", id).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

func (d *WebhookDAO) DeleteSubscription(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := tenant.ApplyFilter(tx.WithContext(ctx), ctx)
		result := scoped.Delete(&localmodel.WebhookSubscription{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrWebhookNotFound
		}
		return tenant.ApplyFilter(tx.WithContext(ctx).Model(&localmodel.WebhookDelivery{}), ctx).Where("subscription_id = ?", id).Delete(&localmodel.WebhookDelivery{}).Error
	})
}

func (d *WebhookDAO) ListDeliveries(ctx context.Context, subscriptionID uint, page, pageSize int) ([]localmodel.WebhookDelivery, int64, error) {
	q := d.scoped(ctx).Model(&localmodel.WebhookDelivery{})
	if subscriptionID > 0 {
		q = q.Where("subscription_id = ?", subscriptionID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []localmodel.WebhookDelivery
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// FanoutOnce converts new audit facts to tenant subscription delivery tasks.
// The cursor and inserts share a transaction; unique(subscription,audit) adds
// another idempotency layer when a worker is interrupted around commit.
func (d *WebhookDAO) FanoutOnce(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = 100
	}
	created := 0
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenants []uint
		// Advance the cursor even while every subscription is disabled; re-enabling
		// a hook must not replay events that occurred during its disabled window.
		if err := tx.Model(&localmodel.WebhookSubscription{}).Distinct("tenant_id").Pluck("tenant_id", &tenants).Error; err != nil {
			return err
		}
		for _, tenantID := range tenants {
			var cursor localmodel.WebhookCursor
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cursor, "tenant_id = ?", tenantID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				cursor = localmodel.WebhookCursor{TenantID: tenantID, UpdatedAt: time.Now()}
				if err = tx.Create(&cursor).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			var logs []localmodel.AuditLog
			if err := tx.Where("tenant_id = ? AND id > ?", tenantID, cursor.LastAuditLogID).Order("id ASC").Limit(batch).Find(&logs).Error; err != nil {
				return err
			}
			if len(logs) == 0 {
				continue
			}
			var subscriptions []localmodel.WebhookSubscription
			if err := tx.Where("tenant_id = ? AND status = ?", tenantID, localmodel.WebhookSubscriptionEnabled).Find(&subscriptions).Error; err != nil {
				return err
			}
			for _, log := range logs {
				for _, subscription := range subscriptions {
					if log.ID <= subscription.StartAuditLogID || !matchesAction(subscription.EventActions, log.Action) {
						continue
					}
					payload := map[string]any{
						"event_id": "audit_" + itoa(tenantID) + "_" + itoa(log.ID),
						"type":     "audit." + log.Action, "action": log.Action,
						"tenant_id": log.TenantID, "actor_type": log.ActorType, "actor_id": log.ActorID,
						"target_type": log.TargetType, "target_id": log.TargetID,
						"summary": log.Summary, "occurred_at": log.CreatedAt.UTC().Format(time.RFC3339Nano),
					}
					delivery := localmodel.WebhookDelivery{TenantID: tenantID, SubscriptionID: subscription.ID, AuditLogID: log.ID, EventID: payload["event_id"].(string), EventAction: log.Action, Payload: payload, Status: localmodel.WebhookDeliveryPending, NextAttemptAt: time.Now()}
					result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery)
					if result.Error != nil {
						return result.Error
					}
					created += int(result.RowsAffected)
				}
			}
			cursor.LastAuditLogID = logs[len(logs)-1].ID
			cursor.UpdatedAt = time.Now()
			if err := tx.Save(&cursor).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

func matchesAction(actions []string, action string) bool {
	for _, candidate := range actions {
		if candidate == "*" || candidate == action {
			return true
		}
	}
	return false
}

func itoa(value uint) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = digits[value%10]
		value /= 10
	}
	return string(buf[pos:])
}

func (d *WebhookDAO) ClaimDeliveries(ctx context.Context, limit int) ([]localmodel.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 20
	}
	if d.db.Dialector.Name() != "postgres" {
		var rows []localmodel.WebhookDelivery
		err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("status IN ? AND next_attempt_at <= ?", []string{localmodel.WebhookDeliveryPending, localmodel.WebhookDeliveryRetrying}, time.Now()).Order("next_attempt_at,id").Limit(limit).Find(&rows).Error; err != nil {
				return err
			}
			for i := range rows {
				rows[i].Attempts++
				rows[i].NextAttemptAt = time.Now().Add(time.Minute)
				if err := tx.Model(&localmodel.WebhookDelivery{}).Where("id = ?", rows[i].ID).Updates(map[string]any{"attempts": rows[i].Attempts, "next_attempt_at": rows[i].NextAttemptAt, "updated_at": time.Now()}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		return rows, err
	}
	var rows []localmodel.WebhookDelivery
	err := d.db.WithContext(ctx).Raw(`WITH claimed AS (
 SELECT id FROM audit_svc.webhook_deliveries WHERE status IN ('pending','retrying') AND next_attempt_at <= NOW()
 ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT ?
) UPDATE audit_svc.webhook_deliveries d SET attempts=d.attempts+1, next_attempt_at=NOW()+INTERVAL '60 seconds', updated_at=NOW()
FROM claimed WHERE d.id=claimed.id RETURNING d.*`, limit).Scan(&rows).Error
	return rows, err
}

func (d *WebhookDAO) MarkSent(ctx context.Context, deliveryID, subscriptionID uint, responseStatus int, body string, at time.Time) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&localmodel.WebhookDelivery{}).Where("id = ?", deliveryID).Updates(map[string]any{"status": localmodel.WebhookDeliverySent, "response_status": responseStatus, "response_body": body, "last_error": "", "delivered_at": at, "updated_at": at}).Error; err != nil {
			return err
		}
		return tx.Model(&localmodel.WebhookSubscription{}).Where("id = ?", subscriptionID).Updates(map[string]any{"consecutive_failures": 0, "last_delivered_at": at, "last_error": "", "updated_at": at}).Error
	})
}

func (d *WebhookDAO) MarkFailed(ctx context.Context, delivery localmodel.WebhookDelivery, responseStatus *int, body, message string, maxAttempts int, next time.Time) error {
	terminal := delivery.Attempts >= maxAttempts
	status := localmodel.WebhookDeliveryRetrying
	if terminal {
		status = localmodel.WebhookDeliveryFailed
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&localmodel.WebhookDelivery{}).Where("id = ?", delivery.ID).Updates(map[string]any{"status": status, "response_status": responseStatus, "response_body": body, "last_error": message, "next_attempt_at": next, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		updates := map[string]any{"last_error": message, "updated_at": time.Now()}
		if terminal {
			updates["consecutive_failures"] = gorm.Expr("consecutive_failures + 1")
			updates["status"] = localmodel.WebhookSubscriptionDisabled
		}
		return tx.Model(&localmodel.WebhookSubscription{}).Where("id = ?", delivery.SubscriptionID).Updates(updates).Error
	})
}
