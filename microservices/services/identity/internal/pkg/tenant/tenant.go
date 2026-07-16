// Package tenant provides multi-tenant context helpers and a GORM isolation plugin (M2).
package tenant

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-admin-kit/services/identity/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContextKey is the request context key for the active tenant id (matches middleware string key).
const ContextKey = "tenant_id"

type disableScopeKey struct{}

// FromContext returns tenant id from context (0 if absent).
func FromContext(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	switch v := ctx.Value(ContextKey).(type) {
	case uint:
		return v
	case uint64:
		return uint(v)
	case int:
		if v > 0 {
			return uint(v)
		}
	case int64:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}

// WithContext stores tenant id on context.
func WithContext(ctx context.Context, tenantID uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ContextKey, tenantID)
}

// DisableScope disables automatic tenant filtering for platform-wide queries (e.g. tenant admin).
func DisableScope(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, disableScopeKey{}, true)
}

func scopeDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(disableScopeKey{}).(bool)
	return v
}

// Normalize maps 0 → 1 (default tenant).
func Normalize(id uint) uint {
	if id == 0 {
		return 1
	}
	return id
}

// Require returns tenant id or error when missing.
func Require(ctx context.Context) (uint, error) {
	id := FromContext(ctx)
	if id == 0 {
		return 0, fmt.Errorf("tenant context required")
	}
	return id, nil
}

var (
	userModelType       = reflect.TypeOf(model.User{})
	roleModelType       = reflect.TypeOf(model.Role{})
	departmentModelType = reflect.TypeOf(model.Department{})
)

const tenantAppliedSetting = "go_admin_kit:tenant_scope_applied"

// Plugin applies tenant_id filters for User / Role / Department.
type Plugin struct{}

func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "go_admin_kit:tenant_scope" }

// Register attaches the tenant isolation plugin to db.
func Register(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("register tenant plugin: db is nil")
	}
	return db.Use(NewPlugin())
}

func (p *Plugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Query().Before("gorm:query").Register("go_admin_kit:tenant:before_query", applyTenantQuery); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("go_admin_kit:tenant:before_row", applyTenantQuery); err != nil {
		return err
	}
	if err := db.Callback().Create().Before("gorm:create").Register("go_admin_kit:tenant:before_create", applyTenantCreate); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("go_admin_kit:tenant:before_update", applyTenantMutate); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("go_admin_kit:tenant:before_delete", applyTenantMutate); err != nil {
		return err
	}
	return nil
}

func applyTenantQuery(db *gorm.DB) {
	if db == nil || db.Error != nil || db.Statement == nil {
		return
	}
	if _, applied := db.Statement.Settings.Load(tenantAppliedSetting); applied {
		return
	}
	ctx := db.Statement.Context
	if scopeDisabled(ctx) {
		return
	}
	tenantID := FromContext(ctx)
	if tenantID == 0 {
		return
	}
	if !ensureSchema(db) || !isTenantModel(db.Statement.Schema.ModelType) {
		return
	}
	db.Statement.Settings.Store(tenantAppliedSetting, true)
	// Use table-qualified column when available.
	col := "tenant_id"
	if db.Statement.Table != "" {
		col = db.Statement.Table + ".tenant_id"
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Name: col}, Value: tenantID},
	}})
}

func applyTenantCreate(db *gorm.DB) {
	if db == nil || db.Error != nil || db.Statement == nil {
		return
	}
	ctx := db.Statement.Context
	if scopeDisabled(ctx) {
		return
	}
	tenantID := FromContext(ctx)
	if tenantID == 0 {
		return
	}
	if !ensureSchema(db) || !isTenantModel(db.Statement.Schema.ModelType) {
		return
	}
	// Reflect set TenantID if zero on dest struct(s)
	setTenantIDOnDest(db.Statement.Dest, tenantID)
	if db.Statement.Schema != nil {
		if field := db.Statement.Schema.LookUpField("TenantID"); field != nil {
			_ = field.Set(db.Statement.Context, db.Statement.ReflectValue, tenantID)
		}
	}
}

func applyTenantMutate(db *gorm.DB) {
	// Updates/deletes: constrain by tenant_id so cross-tenant id guessing fails.
	applyTenantQuery(db)
}

func ensureSchema(db *gorm.DB) bool {
	if db.Statement.Schema != nil {
		return true
	}
	if db.Statement.Model != nil {
		if err := db.Statement.Parse(db.Statement.Model); err != nil {
			return false
		}
	} else if db.Statement.Dest != nil {
		if err := db.Statement.Parse(db.Statement.Dest); err != nil {
			return false
		}
	}
	return db.Statement.Schema != nil
}

func isTenantModel(t reflect.Type) bool {
	if t == nil {
		return false
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t {
	case userModelType, roleModelType, departmentModelType:
		return true
	default:
		return false
	}
}

func setTenantIDOnDest(dest any, tenantID uint) {
	if dest == nil {
		return
	}
	v := reflect.ValueOf(dest)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName("TenantID")
		if f.IsValid() && f.CanSet() && f.Kind() == reflect.Uint && f.Uint() == 0 {
			f.SetUint(uint64(tenantID))
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			for elem.Kind() == reflect.Ptr {
				if elem.IsNil() {
					break
				}
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				f := elem.FieldByName("TenantID")
				if f.IsValid() && f.CanSet() && f.Kind() == reflect.Uint && f.Uint() == 0 {
					f.SetUint(uint64(tenantID))
				}
			}
		}
	}
}
