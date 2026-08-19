package audittrail

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func (p *Plugin) fetchSnapshots(db *gorm.DB, state *mutationState, lock bool) (map[string]map[string]any, map[string]map[string]any, error) {
	model := reflect.New(db.Statement.Schema.ModelType).Interface()
	fields, err := validatedSnapshotFields(db.Statement.Schema, state.target)
	if err != nil {
		return nil, nil, err
	}
	query := db.Session(&gorm.Session{
		NewDB:     true,
		SkipHooks: true,
		Context:   internalContext(db.Statement.Context, state.tenantID),
	}).Model(model).Select(fields)
	query = query.Where(clause.IN{
		Column: clause.Column{Name: state.primaryKey.DBName},
		Values: append([]any(nil), state.primaryIDs...),
	})
	if state.target.TenantField != "" {
		query = query.Where(clause.Eq{Column: clause.Column{Name: state.target.TenantField}, Value: state.tenantID})
	}
	if lock && supportsRowLock(db.Dialector.Name()) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: state.primaryKey.DBName}})
	var rows []map[string]any
	//nolint:unbounded-find -- 查询已由最多 MaxRows 个主键及租户条件显式约束。
	if err := query.Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("capture %s snapshot: %w", state.target.Table, err)
	}
	masked := make(map[string]map[string]any, len(rows))
	raw := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		id := fmt.Sprint(row[state.primaryKey.DBName])
		raw[id] = cloneMap(row)
		masked[id] = redactSnapshot(row, state.target)
	}
	return masked, raw, nil
}

func validatedSnapshotFields(modelSchema *schema.Schema, target Target) ([]string, error) {
	fields := make([]string, 0, len(target.SnapshotFields))
	for _, configured := range target.SnapshotFields {
		field := modelSchema.LookUpField(configured)
		if field == nil || field.DBName == "" {
			return nil, fmt.Errorf("audit target %s model has no field %s", target.Table, configured)
		}
		fields = append(fields, field.DBName)
	}
	return fields, nil
}

func supportsRowLock(dialect string) bool {
	switch strings.ToLower(dialect) {
	case "postgres", "mysql", "sqlserver":
		return true
	default:
		return false
	}
}

func redactSnapshot(row map[string]any, target Target) map[string]any {
	redacted, _ := mask.MaskSensitiveValue(cloneMap(row)).(map[string]any)
	for field, maskType := range target.FieldMasks {
		value, exists := redacted[field]
		if !exists || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		redacted[field] = mask.MaskValue(maskType, text)
	}
	return redacted
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (p *Plugin) insertRecords(db *gorm.DB, state *mutationState, action string, before, after map[string]map[string]any) error {
	ids := state.primaryIDs
	if len(ids) == 0 {
		return fmt.Errorf("%w: %s table=%s has no primary id", ErrAuditConsistency, action, state.target.Table)
	}
	records := make([]auditRecord, 0, len(ids))
	for _, idValue := range ids {
		id := fmt.Sprint(idValue)
		beforeSnapshot := before[id]
		afterSnapshot := after[id]
		if action == "create" && afterSnapshot == nil {
			continue
		}
		if action == "delete" && beforeSnapshot == nil {
			continue
		}
		records = append(records, auditRecord{
			TenantID:   state.tenantID,
			ActorType:  state.actor.Type,
			ActorID:    state.actor.ID,
			Action:     action,
			TargetType: state.target.TargetType,
			TargetID:   id,
			BeforeJSON: beforeSnapshot,
			AfterJSON:  afterSnapshot,
			Summary:    fmt.Sprintf("%s %s", state.target.TargetType, action),
		})
	}
	if len(records) == 0 {
		return fmt.Errorf("%w: %s table=%s produced no audit record", ErrAuditConsistency, action, state.target.Table)
	}
	// Phase 2D/5：优先事务 Outbox（与业务写同事务）→ worker 投 NATS →
	// audit-service 消费落库；未开 Outbox 时直发 NATS；都未配置则同事务直写。
	if outbox.TransactionalEnabled() {
		for i := range records {
			ev := auditevents.AuditEvent{
				TenantID:   records[i].TenantID,
				ActorType:  records[i].ActorType,
				ActorID:    records[i].ActorID,
				Action:     records[i].Action,
				TargetType: records[i].TargetType,
				TargetID:   records[i].TargetID,
				Before:     records[i].BeforeJSON,
				After:      records[i].AfterJSON,
				Summary:    records[i].Summary,
				CreatedAt:  records[i].CreatedAt,
			}
			if ev.CreatedAt.IsZero() {
				ev.CreatedAt = time.Now()
			}
			payload, err := json.Marshal(&ev)
			if err != nil {
				return fmt.Errorf("audit outbox marshal: %w", err)
			}
			subject := "audit.log." + records[i].Action
			if err := outbox.Insert(db, uint64(records[i].TenantID), subject, payload); err != nil {
				return fmt.Errorf("audit outbox insert: %w", err)
			}
		}
		return nil
	}
	if auditevents.Enabled() {
		for i := range records {
			auditevents.Publish(&auditevents.AuditEvent{
				TenantID:   records[i].TenantID,
				ActorType:  records[i].ActorType,
				ActorID:    records[i].ActorID,
				Action:     records[i].Action,
				TargetType: records[i].TargetType,
				TargetID:   records[i].TargetID,
				Before:     records[i].BeforeJSON,
				After:      records[i].AfterJSON,
				Summary:    records[i].Summary,
				CreatedAt:  records[i].CreatedAt,
			})
		}
		return nil
	}
	auditDB := db.Session(&gorm.Session{
		NewDB:     true,
		SkipHooks: true,
		Context:   internalContext(db.Statement.Context, state.tenantID),
	})
	for i := range records {
		if err := auditDB.Create(&records[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
