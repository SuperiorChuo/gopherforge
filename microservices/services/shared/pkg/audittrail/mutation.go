package audittrail

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var rawMutationKeyword = regexp.MustCompile(`(?i)\b(?:DELETE|INSERT|MERGE|REPLACE|TRUNCATE|UPDATE)\b`)

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
