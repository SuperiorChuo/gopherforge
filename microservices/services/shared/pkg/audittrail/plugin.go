package audittrail

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	defaultMaxRows = 1
	stateKey       = "go_admin_kit:audit_trail:state"
)

var (
	// ErrUnboundedMutation rejects audited updates/deletes without a finite
	// primary-key predicate.
	ErrUnboundedMutation = errors.New("audit trail requires a finite primary-key predicate")
	// ErrMutationTooBroad rejects a mutation that exceeds Config.MaxRows.
	ErrMutationTooBroad = errors.New("audit trail mutation exceeds row limit")
	// ErrTenantContextRequired prevents actor-bound tenant data from being
	// attributed to a default tenant by accident.
	ErrTenantContextRequired = errors.New("audit trail tenant context is required")
	// ErrTransactionRequired prevents a business mutation from committing before
	// its audit row when default transactions have been disabled.
	ErrTransactionRequired = errors.New("audit trail requires an active transaction")
	// ErrUnsupportedMutation rejects write shapes that cannot be proven bounded
	// and attributable by the configured model schema.
	ErrUnsupportedMutation = errors.New("audit trail does not support this mutation shape")
	// ErrAuditConsistency indicates that the persisted row could not be matched
	// to the before/after snapshot inside the same transaction.
	ErrAuditConsistency = errors.New("audit trail snapshot is inconsistent with mutation")
)

var safeIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var supportedFieldMasks = map[string]struct{}{
	"email": {},
	"full":  {},
	"hash":  {},
	"ip":    {},
	"path":  {},
	"phone": {},
	"token": {},
}

// Target is one explicitly audited table and its safe snapshot contract.
type Target struct {
	Model          any
	Table          string
	TargetType     string
	TenantField    string
	FixedTenantID  uint
	SnapshotFields []string
	FieldMasks     map[string]string
}

// Config controls the positive audit whitelist and mutation bound.
type Config struct {
	Targets []Target
	MaxRows int
}

// Plugin records create/update/delete snapshots for explicitly configured tables.
type Plugin struct {
	targets        map[string]Target
	targetMentions map[string]*regexp.Regexp
	maxRows        int
}

type mutationState struct {
	owner      *gorm.Statement
	target     Target
	actor      Actor
	tenantID   uint
	primaryKey *schema.Field
	primaryIDs []any
	before     map[string]map[string]any
	beforeRaw  map[string]map[string]any
}

type auditRecord struct {
	ID         uint           `gorm:"primaryKey"`
	TenantID   uint           `gorm:"not null"`
	ActorType  string         `gorm:"not null"`
	ActorID    string         `gorm:"not null"`
	Action     string         `gorm:"not null"`
	TargetType string         `gorm:"not null"`
	TargetID   string         `gorm:"not null"`
	BeforeJSON map[string]any `gorm:"column:before_json;type:json;serializer:json"`
	AfterJSON  map[string]any `gorm:"column:after_json;type:json;serializer:json"`
	Summary    string
	CreatedAt  time.Time
}

func (auditRecord) TableName() string { return "audit_logs" }

// Register attaches a validated audit plugin to db.
func Register(db *gorm.DB, config Config) error {
	if db == nil {
		return errors.New("register audit trail plugin: db is nil")
	}
	plugin, err := NewPlugin(config)
	if err != nil {
		return err
	}
	if err := plugin.validateTargets(db); err != nil {
		return err
	}
	// 表 000072 就绪时全局打开事务 Outbox，使 audittrail 写路径落 public.outbox_events
	// 并由 audit-service worker 投递（crm 等业务方可省去各自 EnableTransactional）。
	if outbox.TableReady(db) && !outbox.TransactionalEnabled() {
		outbox.EnableTransactional()
	}
	return db.Use(plugin)
}

// NewPlugin validates config and creates an audit plugin.
func NewPlugin(config Config) (*Plugin, error) {
	maxRows := config.MaxRows
	if maxRows <= 0 {
		maxRows = defaultMaxRows
	}
	targets := make(map[string]Target, len(config.Targets))
	for _, target := range config.Targets {
		target.Table = strings.ToLower(strings.TrimSpace(target.Table))
		target.TargetType = strings.TrimSpace(target.TargetType)
		target.TenantField = strings.TrimSpace(target.TenantField)
		if target.Model == nil {
			return nil, fmt.Errorf("audit target %s has no model", target.Table)
		}
		if !safeIdentifier.MatchString(target.Table) || target.TargetType == "" {
			return nil, fmt.Errorf("invalid audit target %q/%q", target.Table, target.TargetType)
		}
		if target.Table == (auditRecord{}).TableName() {
			return nil, errors.New("audit_logs cannot be an audit target")
		}
		if target.TenantField != "" && !safeIdentifier.MatchString(target.TenantField) {
			return nil, fmt.Errorf("invalid audit tenant field %q", target.TenantField)
		}
		if len(target.SnapshotFields) == 0 {
			return nil, fmt.Errorf("audit target %s has no snapshot fields", target.Table)
		}
		snapshotFields := make(map[string]struct{}, len(target.SnapshotFields))
		for _, field := range target.SnapshotFields {
			if !safeIdentifier.MatchString(field) {
				return nil, fmt.Errorf("invalid audit snapshot field %q", field)
			}
			snapshotFields[field] = struct{}{}
		}
		for field, maskType := range target.FieldMasks {
			maskType = strings.ToLower(strings.TrimSpace(maskType))
			if !safeIdentifier.MatchString(field) {
				return nil, fmt.Errorf("invalid audit mask field %q", field)
			}
			if _, ok := supportedFieldMasks[maskType]; !ok {
				return nil, fmt.Errorf("invalid audit mask type %q for field %s", maskType, field)
			}
			if _, ok := snapshotFields[field]; !ok {
				return nil, fmt.Errorf("audit mask field %s is not included in snapshots", field)
			}
			target.FieldMasks[field] = maskType
		}
		if _, exists := targets[target.Table]; exists {
			return nil, fmt.Errorf("duplicate audit target %s", target.Table)
		}
		targets[target.Table] = cloneTarget(target)
	}
	targetMentions := make(map[string]*regexp.Regexp, len(targets))
	for table := range targets {
		targetMentions[table] = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])` + regexp.QuoteMeta(table) + `(?:$|[^A-Za-z0-9_])`)
	}
	return &Plugin{targets: targets, targetMentions: targetMentions, maxRows: maxRows}, nil
}

func (p *Plugin) validateTargets(db *gorm.DB) error {
	for _, target := range p.targets {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(target.Model); err != nil {
			return fmt.Errorf("parse audit target %s model: %w", target.Table, err)
		}
		if statement.Schema == nil || statement.Schema.Table != target.Table {
			actual := ""
			if statement.Schema != nil {
				actual = statement.Schema.Table
			}
			return fmt.Errorf("audit target %s model uses table %s", target.Table, actual)
		}
		if len(statement.Schema.PrimaryFields) != 1 {
			return fmt.Errorf("audit target %s requires one primary key", target.Table)
		}
		if _, err := validatedSnapshotFields(statement.Schema, target); err != nil {
			return err
		}
		if target.TenantField != "" {
			field := statement.Schema.LookUpField(target.TenantField)
			if field == nil || field.DBName != target.TenantField {
				return fmt.Errorf("audit target %s model has no tenant field %s", target.Table, target.TenantField)
			}
		}
	}
	return nil
}

func cloneTarget(target Target) Target {
	target.SnapshotFields = append([]string(nil), target.SnapshotFields...)
	if target.FieldMasks != nil {
		masks := make(map[string]string, len(target.FieldMasks))
		for field, maskType := range target.FieldMasks {
			masks[field] = maskType
		}
		target.FieldMasks = masks
	}
	return target
}

func (p *Plugin) Name() string { return "go_admin_kit:audit_trail" }

// Initialize installs callbacks inside GORM's default transaction, before its
// final commit-or-rollback callback.
func (p *Plugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().After("gorm:begin_transaction").Before("gorm:before_create").Register("go_admin_kit:audit_trail:before_create", p.beforeCreate); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:after_create").Before("gorm:commit_or_rollback_transaction").Register("go_admin_kit:audit_trail:after_create", p.afterCreate); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:setup_reflect_value").Before("gorm:before_update").Register("go_admin_kit:audit_trail:before_update", p.beforeUpdate); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:after_update").Before("gorm:commit_or_rollback_transaction").Register("go_admin_kit:audit_trail:after_update", p.afterUpdate); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:begin_transaction").Before("gorm:before_delete").Register("go_admin_kit:audit_trail:before_delete", p.beforeDelete); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:after_delete").Before("gorm:commit_or_rollback_transaction").Register("go_admin_kit:audit_trail:after_delete", p.afterDelete); err != nil {
		return err
	}
	return db.Callback().Raw().Before("gorm:raw").Register("go_admin_kit:audit_trail:reject_raw_mutation", p.rejectRawMutation)
}
