package audittrail

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

var rawANDSeparator = regexp.MustCompile(`(?i)\s+AND\s+`)
var rawPredicate = regexp.MustCompile("(?i)^(?:[A-Za-z_][A-Za-z0-9_]*\\.)?[\"`]?([A-Za-z_][A-Za-z0-9_]*)[\"`]?\\s*(=|IN)\\s*(?:\\(\\s*)?\\?(?:\\s*\\))?$")

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
