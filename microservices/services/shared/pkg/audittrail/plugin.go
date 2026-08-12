package audittrail

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
var rawANDSeparator = regexp.MustCompile(`(?i)\s+AND\s+`)
var rawPredicate = regexp.MustCompile("(?i)^(?:[A-Za-z_][A-Za-z0-9_]*\\.)?[\"`]?([A-Za-z_][A-Za-z0-9_]*)[\"`]?\\s*(=|IN)\\s*(?:\\(\\s*)?\\?(?:\\s*\\))?$")
var rawMutationKeyword = regexp.MustCompile(`(?i)\b(?:DELETE|INSERT|MERGE|REPLACE|TRUNCATE|UPDATE)\b`)

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

func (p *Plugin) rejectRawMutation(db *gorm.DB) {
	if db == nil || db.Error != nil || db.Statement == nil || db.DryRun || isDisabled(db.Statement.Context) {
		return
	}
	if _, ok := ActorFromContext(db.Statement.Context); !ok {
		return
	}
	sqlText := db.Statement.SQL.String()
	if !rawMutationKeyword.MatchString(sqlText) {
		return
	}
	for table, matcher := range p.targetMentions {
		if matcher.MatchString(sqlText) {
			db.AddError(fmt.Errorf("%w: raw SQL references audited table %s", ErrUnsupportedMutation, table))
			return
		}
	}
}

func (p *Plugin) beforeCreate(db *gorm.DB) {
	state, ok := p.newState(db)
	if !ok {
		return
	}
	if _, conflict := db.Statement.Clauses["ON CONFLICT"]; conflict {
		db.AddError(fmt.Errorf("%w: audited upsert on %s", ErrUnboundedMutation, state.target.Table))
		return
	}
	if !isSupportedCreateValue(db.Statement.ReflectValue) {
		db.AddError(fmt.Errorf("%w: audited create on %s requires struct values", ErrUnsupportedMutation, state.target.Table))
		return
	}
	count := mutationValueCount(db.Statement.ReflectValue)
	if count > p.maxRows {
		db.AddError(fmt.Errorf("%w: table=%s rows=%d limit=%d", ErrMutationTooBroad, state.target.Table, count, p.maxRows))
		return
	}
	if state.target.TenantField != "" {
		setTenantField(db, state.target.TenantField, state.tenantID)
	}
	db.Statement.Settings.Store(stateKey, state)
}

func (p *Plugin) afterCreate(db *gorm.DB) {
	state, ok := loadState(db)
	if !ok || db.Error != nil || db.RowsAffected == 0 {
		return
	}
	ids, err := primaryIDsFromValue(db, db.Statement.ReflectValue, state.primaryKey)
	if err != nil {
		db.AddError(err)
		return
	}
	if len(ids) == 0 || len(ids) > p.maxRows {
		db.AddError(fmt.Errorf("%w: create table=%s ids=%d", ErrAuditConsistency, state.target.Table, len(ids)))
		return
	}
	state.primaryIDs = ids
	after, _, err := p.fetchSnapshots(db, state, false)
	if err != nil {
		db.AddError(err)
		return
	}
	if int64(len(after)) != db.RowsAffected {
		db.AddError(fmt.Errorf("%w: create table=%s affected=%d snapshots=%d", ErrAuditConsistency, state.target.Table, db.RowsAffected, len(after)))
		return
	}
	if err := p.insertRecords(db, state, "create", nil, after); err != nil {
		db.AddError(err)
	}
}

func (p *Plugin) beforeUpdate(db *gorm.DB) {
	p.beforeMutation(db)
}

func (p *Plugin) beforeDelete(db *gorm.DB) {
	p.beforeMutation(db)
}

func (p *Plugin) beforeMutation(db *gorm.DB) {
	state, ok := p.newState(db)
	if !ok {
		return
	}
	if state.target.TenantField != "" {
		db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: state.target.TenantField}, Value: state.tenantID},
		}})
	}
	ids, err := boundedPrimaryIDs(db, state.primaryKey)
	if err != nil {
		db.AddError(err)
		return
	}
	if len(ids) > p.maxRows {
		db.AddError(fmt.Errorf("%w: table=%s rows=%d limit=%d", ErrMutationTooBroad, state.target.Table, len(ids), p.maxRows))
		return
	}
	state.primaryIDs = ids
	state.before, state.beforeRaw, err = p.fetchSnapshots(db, state, true)
	if err != nil {
		db.AddError(err)
		return
	}
	if len(state.before) != len(state.primaryIDs) {
		db.AddError(fmt.Errorf("%w: table=%s requested=%d visible=%d", ErrAuditConsistency, state.target.Table, len(state.primaryIDs), len(state.before)))
		return
	}
	db.Statement.Settings.Store(stateKey, state)
}

func (p *Plugin) afterUpdate(db *gorm.DB) {
	state, ok := loadState(db)
	if !ok || db.Error != nil || db.RowsAffected == 0 {
		return
	}
	after, afterRaw, err := p.fetchSnapshots(db, state, false)
	if err != nil {
		db.AddError(err)
		return
	}
	if int64(len(state.before)) != db.RowsAffected || len(after) != len(state.before) {
		db.AddError(fmt.Errorf("%w: update table=%s affected=%d", ErrAuditConsistency, state.target.Table, db.RowsAffected))
		return
	}
	for id := range state.beforeRaw {
		if _, exists := afterRaw[id]; !exists {
			db.AddError(fmt.Errorf("%w: update table=%s id=%s", ErrAuditConsistency, state.target.Table, id))
			return
		}
	}
	state.primaryIDs = existingPrimaryIDs(state.primaryIDs, state.before)
	if err := p.insertRecords(db, state, "update", state.before, after); err != nil {
		db.AddError(err)
	}
}

func (p *Plugin) afterDelete(db *gorm.DB) {
	state, ok := loadState(db)
	if !ok || db.Error != nil || db.RowsAffected == 0 {
		return
	}
	if int64(len(state.before)) != db.RowsAffected {
		db.AddError(fmt.Errorf("%w: delete table=%s affected=%d", ErrAuditConsistency, state.target.Table, db.RowsAffected))
		return
	}
	remaining, _, err := p.fetchSnapshots(db, state, false)
	if err != nil {
		db.AddError(err)
		return
	}
	if len(remaining) != 0 {
		db.AddError(fmt.Errorf("%w: deleted row still present table=%s", ErrAuditConsistency, state.target.Table))
		return
	}
	state.primaryIDs = existingPrimaryIDs(state.primaryIDs, state.before)
	if err := p.insertRecords(db, state, "delete", state.before, nil); err != nil {
		db.AddError(err)
	}
}

func (p *Plugin) newState(db *gorm.DB) (*mutationState, bool) {
	if db == nil || db.Error != nil || db.Statement == nil || db.DryRun || isDisabled(db.Statement.Context) {
		return nil, false
	}
	actor, ok := ActorFromContext(db.Statement.Context)
	if !ok {
		return nil, false
	}
	target, ok, err := p.targetFor(db.Statement)
	if err != nil {
		db.AddError(err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		db.AddError(fmt.Errorf("%w: table=%s actor=%s", ErrTransactionRequired, target.Table, actor.ID))
		return nil, false
	}
	if db.Statement.Schema == nil || len(db.Statement.Schema.PrimaryFields) != 1 {
		db.AddError(fmt.Errorf("%w: table=%s requires one primary key", ErrUnboundedMutation, target.Table))
		return nil, false
	}
	tenantID := target.FixedTenantID
	if tenantID == 0 {
		var found bool
		tenantID, found = TenantIDFromContext(db.Statement.Context)
		if !found {
			db.AddError(fmt.Errorf("%w: table=%s actor=%s", ErrTenantContextRequired, target.Table, actor.ID))
			return nil, false
		}
	}
	return &mutationState{
		owner:      db.Statement,
		target:     target,
		actor:      actor,
		tenantID:   tenantID,
		primaryKey: db.Statement.Schema.PrimaryFields[0],
	}, true
}

func (p *Plugin) targetFor(stmt *gorm.Statement) (Target, bool, error) {
	if stmt == nil {
		return Target{}, false, nil
	}
	table := strings.ToLower(strings.TrimSpace(stmt.Table))
	schemaTable := ""
	if stmt.Schema != nil {
		schemaTable = strings.ToLower(strings.TrimSpace(stmt.Schema.Table))
	}
	if schemaTarget, ok := p.targets[schemaTable]; ok {
		if table == "" || table == schemaTable {
			return schemaTarget, true, nil
		}
		return Target{}, false, fmt.Errorf("%w: audited model %s uses table expression %q", ErrUnsupportedMutation, schemaTable, stmt.Table)
	}
	if target, ok := p.targets[table]; ok {
		if stmt.Schema == nil || schemaTable != table {
			return Target{}, false, fmt.Errorf("%w: audited table %s requires its configured model schema", ErrUnsupportedMutation, target.Table)
		}
		return target, true, nil
	}
	for name, target := range p.targets {
		mentioned := p.targetMentions[name].MatchString(table)
		if mentioned {
			return Target{}, false, fmt.Errorf("%w: unsupported audited table expression %q for %s", ErrUnsupportedMutation, stmt.Table, target.Table)
		}
	}
	return Target{}, false, nil
}

func loadState(db *gorm.DB) (*mutationState, bool) {
	if db == nil || db.Statement == nil {
		return nil, false
	}
	value, ok := db.Statement.Settings.Load(stateKey)
	if !ok {
		return nil, false
	}
	state, ok := value.(*mutationState)
	return state, ok && state != nil && state.owner == db.Statement
}

func mutationValueCount(value reflect.Value) int {
	for value.IsValid() && (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return 0
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		return value.Len()
	}
	return 1
}

func isSupportedCreateValue(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			if !isSupportedCreateValue(value.Index(i)) {
				return false
			}
		}
		return value.Len() > 0
	}
	return value.Kind() == reflect.Struct
}

func setTenantField(db *gorm.DB, fieldName string, tenantID uint) {
	if db.Statement.Schema == nil {
		return
	}
	field := db.Statement.Schema.LookUpField(fieldName)
	if field == nil {
		db.AddError(fmt.Errorf("audit target %s has no tenant field %s", db.Statement.Table, fieldName))
		return
	}
	value := db.Statement.ReflectValue
	for value.IsValid() && (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) {
		value = value.Elem()
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			if err := field.Set(db.Statement.Context, value.Index(i), tenantID); err != nil {
				db.AddError(err)
				return
			}
		}
		return
	}
	if value.IsValid() {
		db.AddError(field.Set(db.Statement.Context, value, tenantID))
	}
}

func primaryIDsFromValue(db *gorm.DB, value reflect.Value, primaryKey *schema.Field) ([]any, error) {
	if primaryKey == nil || !value.IsValid() {
		return nil, nil
	}
	probe := value
	for probe.IsValid() && (probe.Kind() == reflect.Ptr || probe.Kind() == reflect.Interface) {
		if probe.IsNil() {
			return nil, nil
		}
		probe = probe.Elem()
	}
	if !probe.IsValid() {
		return nil, nil
	}
	_, values := schema.GetIdentityFieldValuesMap(db.Statement.Context, value, []*schema.Field{primaryKey})
	ids := make([]any, 0, len(values))
	for _, valueSet := range values {
		if len(valueSet) == 1 && !isZeroPrimaryID(valueSet[0]) {
			ids = append(ids, valueSet[0])
		}
	}
	return dedupeIDs(ids), nil
}

func boundedPrimaryIDs(db *gorm.DB, primaryKey *schema.Field) ([]any, error) {
	for _, value := range []reflect.Value{reflect.ValueOf(db.Statement.Model), db.Statement.ReflectValue} {
		ids, err := primaryIDsFromValue(db, value, primaryKey)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			if where, ok := db.Statement.Clauses["WHERE"]; ok && where.Expression != nil && !mutationFilterIsConjunctive(where.Expression) {
				return nil, ErrUnboundedMutation
			}
			return ids, nil
		}
	}
	where, ok := db.Statement.Clauses["WHERE"]
	if !ok || where.Expression == nil {
		return nil, ErrUnboundedMutation
	}
	ids, bounded := primaryIDsFromExpression(where.Expression, primaryKey.DBName)
	ids = dedupeIDs(ids)
	if !bounded || len(ids) == 0 {
		return nil, ErrUnboundedMutation
	}
	return ids, nil
}

func mutationFilterIsConjunctive(expression clause.Expression) bool {
	switch expr := expression.(type) {
	case clause.Where:
		return conjunctionIsSafe(expr.Exprs)
	case clause.AndConditions:
		return conjunctionIsSafe(expr.Exprs)
	case clause.OrConditions:
		if len(expr.Exprs) < 2 {
			return false
		}
		for _, child := range expr.Exprs {
			if !mutationFilterIsConjunctive(child) {
				return false
			}
		}
		return true
	case clause.Eq, clause.IN:
		return true
	case clause.Expr:
		return rawSQLIsConjunctive(expr.SQL, expr.Vars)
	default:
		return false
	}
}

func conjunctionIsSafe(expressions []clause.Expression) bool {
	if len(expressions) == 0 {
		return false
	}
	for _, expression := range expressions {
		if or, ok := expression.(clause.OrConditions); ok && len(or.Exprs) == 1 {
			return false
		}
		if !mutationFilterIsConjunctive(expression) {
			return false
		}
	}
	return true
}

func primaryIDsFromExpression(expression clause.Expression, primaryKey string) ([]any, bool) {
	switch expr := expression.(type) {
	case clause.Where:
		return primaryIDsFromConjunction(expr.Exprs, primaryKey)
	case clause.AndConditions:
		return primaryIDsFromConjunction(expr.Exprs, primaryKey)
	case clause.OrConditions:
		var ids []any
		for _, child := range expr.Exprs {
			childIDs, bounded := primaryIDsFromExpression(child, primaryKey)
			if !bounded {
				return nil, false
			}
			ids = append(ids, childIDs...)
		}
		return ids, len(ids) > 0
	case clause.Eq:
		if columnMatchesPrimaryKey(expr.Column, primaryKey) {
			return flattenIDValue(expr.Value), true
		}
	case clause.IN:
		if columnMatchesPrimaryKey(expr.Column, primaryKey) {
			return flattenIDValue(expr.Values), true
		}
	case clause.Expr:
		return primaryIDsFromSQLExpression(expr.SQL, expr.Vars, primaryKey)
	}
	return nil, false
}

func primaryIDsFromConjunction(expressions []clause.Expression, primaryKey string) ([]any, bool) {
	// GORM renders a single-child OrConditions after another WHERE expression
	// with an OR separator, even when it sits inside clause.Where or
	// clause.AndConditions. Reject that shape instead of treating it as another
	// conjunct and extracting an unrelated primary-key bound.
	if len(expressions) > 1 {
		for _, expression := range expressions {
			if or, ok := expression.(clause.OrConditions); ok && len(or.Exprs) == 1 {
				return nil, false
			}
		}
	}
	for _, expression := range expressions {
		if ids, bounded := primaryIDsFromExpression(expression, primaryKey); bounded {
			return ids, true
		}
	}
	return nil, false
}

func primaryIDsFromSQLExpression(sqlText string, vars []any, primaryKey string) ([]any, bool) {
	predicates, ok := rawSQLPredicates(sqlText, vars)
	if !ok {
		return nil, false
	}
	var ids []any
	for i, predicate := range predicates {
		match := rawPredicate.FindStringSubmatch(strings.TrimSpace(predicate))
		if strings.EqualFold(match[1], primaryKey) {
			if len(ids) != 0 {
				return nil, false
			}
			ids = flattenIDValue(vars[i])
		}
	}
	return ids, len(ids) > 0
}

func rawSQLIsConjunctive(sqlText string, vars []any) bool {
	_, ok := rawSQLPredicates(sqlText, vars)
	return ok
}

func rawSQLPredicates(sqlText string, vars []any) ([]string, bool) {
	if strings.Contains(sqlText, ";") || strings.Contains(sqlText, "--") || strings.Contains(sqlText, "/*") || strings.Contains(sqlText, "*/") {
		return nil, false
	}
	predicates := rawANDSeparator.Split(strings.TrimSpace(sqlText), -1)
	if len(predicates) == 0 || len(predicates) != len(vars) {
		return nil, false
	}
	for _, predicate := range predicates {
		match := rawPredicate.FindStringSubmatch(strings.TrimSpace(predicate))
		if len(match) != 3 {
			return nil, false
		}
	}
	return predicates, true
}

func columnMatchesPrimaryKey(column any, primaryKey string) bool {
	switch value := column.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(value), primaryKey) || value == clause.PrimaryKey
	case clause.Column:
		return strings.EqualFold(value.Name, primaryKey) || value.Name == clause.PrimaryKey
	default:
		return false
	}
}

func flattenIDValue(value any) []any {
	if value == nil {
		return nil
	}
	reflected := reflect.ValueOf(value)
	for reflected.IsValid() && (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface) {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
	}
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		if isZeroPrimaryID(value) {
			return nil
		}
		return []any{value}
	}
	ids := make([]any, 0, reflected.Len())
	for i := 0; i < reflected.Len(); i++ {
		item := reflected.Index(i).Interface()
		if !isZeroPrimaryID(item) {
			ids = append(ids, item)
		}
	}
	return ids
}

func dedupeIDs(ids []any) []any {
	seen := make(map[string]struct{}, len(ids))
	result := make([]any, 0, len(ids))
	for _, id := range ids {
		key := fmt.Sprintf("%T:%v", id, id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, id)
	}
	return result
}

func existingPrimaryIDs(ids []any, snapshots map[string]map[string]any) []any {
	existing := make([]any, 0, len(snapshots))
	for _, id := range ids {
		if _, ok := snapshots[fmt.Sprint(id)]; ok {
			existing = append(existing, id)
		}
	}
	return existing
}

func isZeroPrimaryID(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	for reflected.IsValid() && (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface) {
		if reflected.IsNil() {
			return true
		}
		reflected = reflected.Elem()
	}
	return reflected.IsValid() && reflected.IsZero()
}

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
