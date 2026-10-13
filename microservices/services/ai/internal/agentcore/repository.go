package agentcore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db  *gorm.DB
	now func() time.Time
}

// maxListedVersions 限制单个 Agent 的版本列表行数（展示用）；发布频率受写权限约束，
// 历史版本行按 ID 引用不受此限制影响。
const maxListedVersions = 100

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db, now: time.Now}
}

func (r *Repository) DB() *gorm.DB { return r.db }

func (r *Repository) AutoMigrateForTest() error {
	return r.db.AutoMigrate(
		&AgentDefinition{}, &AgentVersion{}, &AgentVersionTool{}, &AgentRun{},
		&AgentStep{}, &AgentToolCall{}, &RunEvent{},
	)
}

func tenantIDFromContext(ctx context.Context) (uint, error) {
	tenantID, err := tenant.Require(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: tenant context required", ErrValidation)
	}
	return tenantID, nil
}

func normalizeDBError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func pageBounds(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (r *Repository) CreateDefinition(ctx context.Context, definition *AgentDefinition) error {
	if definition == nil {
		return ErrValidation
	}
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return err
	}
	definition.TenantID = tenantID
	definition.CreatedAt = r.now().UTC()
	definition.UpdatedAt = definition.CreatedAt
	if err := r.db.WithContext(ctx).Create(definition).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent key already exists", ErrConflict)
		}
		return err
	}
	return nil
}

func (r *Repository) GetDefinition(ctx context.Context, id uint64) (*AgentDefinition, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var definition AgentDefinition
	err = r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&definition).Error
	if err != nil {
		return nil, normalizeDBError(err)
	}
	return &definition, nil
}

func (r *Repository) ListDefinitions(ctx context.Context, page, pageSize int) ([]AgentDefinition, int64, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize = pageBounds(page, pageSize)
	query := r.db.WithContext(ctx).Model(&AgentDefinition{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var definitions []AgentDefinition
	err = query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&definitions).Error
	return definitions, total, err
}

func (r *Repository) UpdateDefinition(ctx context.Context, id uint64, name, description, status string) (*AgentDefinition, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	result := r.db.WithContext(ctx).Model(&AgentDefinition{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]any{"name": name, "description": description, "status": status, "updated_at": r.now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	return r.GetDefinition(ctx, id)
}

func (r *Repository) PublishVersion(
	ctx context.Context,
	definitionID, actorID uint64,
	spec AgentSpec,
	specJSON []byte,
	specHash string,
	toolHashes map[string]string,
) (*AgentVersion, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if spec.Tools == nil {
		spec.Tools = []string{}
	}
	if !validDatabaseID(definitionID) || !validDatabaseID(actorID) || !validDatabaseID(spec.ModelID) || spec.Schema != AgentSpecSchema || spec.Name == "" || utf8.RuneCountInString(spec.Name) > 100 || spec.Instructions == "" || utf8.RuneCountInString(spec.Instructions) > 20_000 || spec.MaxSteps < 1 || spec.MaxSteps > 32 || spec.TimeoutSeconds < 10 || spec.TimeoutSeconds > 600 || !validSHA256(specHash) {
		return nil, ErrValidation
	}
	for _, name := range spec.Tools {
		if !toolNamePattern.MatchString(name) || !validSHA256(toolHashes[name]) {
			return nil, ErrValidation
		}
	}
	if len(toolHashes) != len(spec.Tools) {
		return nil, ErrValidation
	}
	expectedHash, expectedJSON, hashErr := hashJSON(spec)
	if hashErr != nil || expectedHash != specHash || !bytes.Equal(expectedJSON, specJSON) {
		return nil, ErrValidation
	}
	toolsJSON, err := NewJSONValue(spec.Tools)
	if err != nil {
		return nil, err
	}
	var version AgentVersion
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var definition AgentDefinition
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND id = ?", tenantID, definitionID)
		if err := query.First(&definition).Error; err != nil {
			return normalizeDBError(err)
		}
		var latest int
		if err := tx.Model(&AgentVersion{}).
			Where("tenant_id = ? AND agent_definition_id = ?", tenantID, definitionID).
			Select("COALESCE(MAX(version), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		version = AgentVersion{
			TenantID: tenantID, AgentDefinitionID: definitionID, Version: latest + 1,
			SchemaVersion: AgentSpecSchema, ModelID: spec.ModelID, Instructions: spec.Instructions,
			Tools: toolsJSON, MaxSteps: spec.MaxSteps, TimeoutSeconds: spec.TimeoutSeconds,
			Spec: JSONValue(append([]byte(nil), specJSON...)), SpecSHA256: specHash, CreatedBy: actorID, CreatedAt: r.now().UTC(),
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		for _, toolName := range spec.Tools {
			grant := AgentVersionTool{
				TenantID: tenantID, AgentVersionID: version.ID, ToolName: toolName,
				ToolSpecSHA256: toolHashes[toolName], CreatedAt: r.now().UTC(),
			}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&AgentDefinition{}).
			Where("tenant_id = ? AND id = ?", tenantID, definitionID).
			Updates(map[string]any{"latest_version_id": version.ID, "updated_at": r.now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *Repository) GetVersion(ctx context.Context, id uint64) (*AgentVersion, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var version AgentVersion
	err = r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&version).Error
	if err != nil {
		return nil, normalizeDBError(err)
	}
	return &version, nil
}

func (r *Repository) ListVersions(ctx context.Context, definitionID uint64) ([]AgentVersion, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var versions []AgentVersion
	err = r.db.WithContext(ctx).
		Where("tenant_id = ? AND agent_definition_id = ?", tenantID, definitionID).
		Order("version DESC").Limit(maxListedVersions).Find(&versions).Error
	return versions, err
}

// CreateRun persists the idempotency decision and queued event in one transaction.
func (r *Repository) CreateRun(ctx context.Context, run AgentRun) (*AgentRun, bool, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, false, err
	}
	run.TenantID = tenantID
	now := r.now().UTC()
	run.CreatedAt, run.UpdatedAt = now, now
	var existing AgentRun
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("tenant_id = ? AND requested_by = ? AND idempotency_key = ?", tenantID, run.RequestedBy, run.IdempotencyKey)
		if findErr := query.First(&existing).Error; findErr == nil {
			return nil
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if createErr := tx.Create(&run).Error; createErr != nil {
			return createErr
		}
		return appendEvent(tx, tenantID, run.ID, nil, "run.queued", map[string]any{"attempt": 0}, now)
	})
	if err != nil {
		if isUniqueViolation(err) {
			findErr := r.db.WithContext(ctx).
				Where("tenant_id = ? AND requested_by = ? AND idempotency_key = ?", tenantID, run.RequestedBy, run.IdempotencyKey).
				First(&existing).Error
			if findErr != nil {
				return nil, false, err
			}
		} else {
			return nil, false, err
		}
	}
	if existing.ID != 0 {
		if !sameRunRequest(existing, run) {
			return nil, false, ErrIdempotencyConflict
		}
		return &existing, true, nil
	}
	return &run, false, nil
}

func (r *Repository) GetRun(ctx context.Context, id uint64) (*AgentRun, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var run AgentRun
	err = r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&run).Error
	if err != nil {
		return nil, normalizeDBError(err)
	}
	return &run, nil
}

func (r *Repository) ListRuns(ctx context.Context, definitionID uint64, status string, page, pageSize int) ([]AgentRun, int64, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize = pageBounds(page, pageSize)
	query := r.db.WithContext(ctx).Model(&AgentRun{}).Where("tenant_id = ?", tenantID)
	if definitionID != 0 {
		query = query.Where("agent_definition_id = ?", definitionID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []AgentRun
	err = query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&runs).Error
	return runs, total, err
}

func (r *Repository) RequestCancel(ctx context.Context, id uint64) (*AgentRun, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	now := r.now().UTC()
	var result AgentRun
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND id = ?", tenantID, id).First(&result).Error; err != nil {
			return normalizeDBError(err)
		}
		if result.IsTerminal() || result.CancelRequestedAt != nil {
			return nil
		}
		updates := map[string]any{"cancel_requested_at": now, "updated_at": now}
		if result.Status == RunQueued {
			updates["status"] = RunCancelled
			updates["finished_at"] = now
			updates["lease_owner"] = ""
			updates["lease_expires_at"] = nil
			result.Status = RunCancelled
		} else {
			updates["status"] = RunCancelling
			result.Status = RunCancelling
		}
		if err := tx.Model(&AgentRun{}).Where("tenant_id = ? AND id = ?", tenantID, id).Updates(updates).Error; err != nil {
			return err
		}
		result.CancelRequestedAt = &now
		if err := appendEvent(tx, tenantID, id, nil, "run.cancel_requested", map[string]any{}, now); err != nil {
			return err
		}
		if result.Status == RunCancelled {
			return appendEvent(tx, tenantID, id, nil, "run.cancelled", map[string]any{"before_start": true}, now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *Repository) ListEventsAfter(ctx context.Context, runID, afterID uint64, limit int) ([]RunEvent, bool, error) {
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	var events []RunEvent
	err = r.db.WithContext(ctx).
		Where("tenant_id = ? AND run_id = ? AND id > ?", run.TenantID, runID, afterID).
		Order("id ASC").Limit(limit).Find(&events).Error
	return events, run.IsTerminal(), err
}

func appendEvent(tx *gorm.DB, tenantID uint, runID uint64, stepID *uint64, eventType string, data any, at time.Time) error {
	payload, err := NewJSONValue(data)
	if err != nil {
		return err
	}
	return tx.Create(&RunEvent{
		TenantID: tenantID, RunID: runID, StepID: stepID, SchemaVersion: RunEventSchema,
		EventType: eventType, Data: payload, OccurredAt: at,
	}).Error
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}
