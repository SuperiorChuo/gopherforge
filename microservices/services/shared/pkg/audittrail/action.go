package audittrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/gorm"
)

// ActionRecord describes a security-sensitive read/action which cannot be
// captured by the GORM mutation callbacks (for example, private-key export).
// Metadata must contain operational facts only; it is masked again before
// persistence as a defence in depth.
type ActionRecord struct {
	TenantID   uint
	Action     string
	TargetType string
	TargetID   string
	Summary    string
	Metadata   map[string]any
}

// RecordAction synchronously persists an action audit row. The caller must
// wait for this function to succeed before releasing the sensitive result.
// Unlike request operation logs this path never queues or silently drops.
func RecordAction(ctx context.Context, db *gorm.DB, record ActionRecord) error {
	if db == nil {
		return errors.New("record audit action: db is nil")
	}
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return ErrTenantContextRequired
	}
	tenantID := record.TenantID
	if tenantID == 0 {
		var found bool
		tenantID, found = TenantIDFromContext(ctx)
		if !found {
			return ErrTenantContextRequired
		}
	}
	action := strings.TrimSpace(record.Action)
	targetType := strings.TrimSpace(record.TargetType)
	targetID := strings.TrimSpace(record.TargetID)
	if action == "" || targetType == "" || targetID == "" {
		return errors.New("record audit action: action and target are required")
	}

	metadata := map[string]any{}
	if record.Metadata != nil {
		masked, ok := mask.MaskSensitiveValue(record.Metadata).(map[string]any)
		if !ok {
			return errors.New("record audit action: invalid metadata")
		}
		metadata = masked
	}
	summary := strings.TrimSpace(record.Summary)
	if summary == "" {
		summary = fmt.Sprintf("%s %s", targetType, action)
	}

	createdAt := time.Now().UTC()
	row := auditRecord{
		TenantID:   tenantID,
		ActorType:  actor.Type,
		ActorID:    actor.ID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		AfterJSON:  metadata,
		Summary:    summary,
		CreatedAt:  createdAt,
	}
	// In production the audit service owns persistence. Insert the action into
	// the transactional outbox and wait for its commit before releasing the
	// sensitive result; unlike PublishAsync this path can fail closed.
	if outbox.TransactionalEnabled() {
		event := auditevents.AuditEvent{
			TenantID: tenantID, ActorType: actor.Type, ActorID: actor.ID,
			Action: action, TargetType: targetType, TargetID: targetID,
			After: metadata, Summary: summary, CreatedAt: createdAt,
		}
		payload, err := json.Marshal(&event)
		if err != nil {
			return fmt.Errorf("record audit action: marshal outbox event: %w", err)
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return outbox.Insert(tx, uint64(tenantID), "audit.log."+action, payload)
		})
	}

	return db.Session(&gorm.Session{
		NewDB:     true,
		SkipHooks: true,
		Context:   internalContext(ctx, tenantID),
	}).Create(&row).Error
}
