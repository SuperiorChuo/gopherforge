package authz

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/gorm"
)

type dataScopeDirectiveContextKey struct{}

type dataScopeDirective struct {
	Disabled bool
	Scope    UserDataScope
}

var (
	userModelType         = reflect.TypeOf(model.User{})
	fileModelType         = reflect.TypeOf(model.File{})
	loginLogModelType     = reflect.TypeOf(model.LoginLog{})
	operationLogModelType = reflect.TypeOf(model.OperationLog{})
)

const dataScopeAppliedSetting = "go_admin_kit:data_scope_applied"

// EnableDataScope marks a query context for plugin-managed data-scope filtering.
func EnableDataScope(ctx context.Context, scope UserDataScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, dataScopeDirectiveContextKey{}, dataScopeDirective{
		Scope: scope,
	})
}

// DisableDataScope explicitly disables plugin-managed data-scope filtering.
func DisableDataScope(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, dataScopeDirectiveContextKey{}, dataScopeDirective{
		Disabled: true,
	})
}

// ForceSelfScope forces plugin-managed queries to use self scope for the supplied user ID.
func ForceSelfScope(ctx context.Context, userID uint) context.Context {
	return EnableDataScope(ctx, UserDataScope{
		Scope:  DataScopeSelf,
		UserID: userID,
	})
}

// DataScopePlugin applies opt-in data-scope filters for supported models.
type DataScopePlugin struct{}

func NewDataScopePlugin() *DataScopePlugin {
	return &DataScopePlugin{}
}

func RegisterDataScopePlugin(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("register data scope plugin: db is nil")
	}
	return db.Use(NewDataScopePlugin())
}

func (p *DataScopePlugin) Name() string {
	return "go_admin_kit:data_scope"
}

func (p *DataScopePlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Query().Before("gorm:query").Register("go_admin_kit:data_scope:before_query", applyDataScopePlugin); err != nil {
		return err
	}
	return db.Callback().Row().Before("gorm:row").Register("go_admin_kit:data_scope:before_row", applyDataScopePlugin)
}

func applyDataScopePlugin(db *gorm.DB) {
	if db == nil || db.Error != nil || db.Statement == nil {
		return
	}
	if _, applied := db.Statement.Settings.Load(dataScopeAppliedSetting); applied {
		return
	}

	directive, ok := lookupDataScopeDirective(db.Statement.Context)
	if !ok || directive.Disabled {
		return
	}

	if db.Statement.Schema == nil {
		if db.Statement.Model != nil {
			if err := db.Statement.Parse(db.Statement.Model); err != nil {
				return
			}
		} else if db.Statement.Dest != nil {
			if err := db.Statement.Parse(db.Statement.Dest); err != nil {
				return
			}
		}
	}
	if db.Statement.Schema == nil {
		return
	}
	if !isSimpleDataScopeTarget(db.Statement) {
		return
	}

	var scoped *gorm.DB
	switch db.Statement.Schema.ModelType {
	case userModelType:
		scoped = ApplyUserEntityScope(db, directive.Scope, "id", "department_id")
	case fileModelType, loginLogModelType, operationLogModelType:
		scoped = ApplyOwnerScope(db, directive.Scope, "user_id")
	default:
		return
	}

	if scoped != nil && scoped != db {
		*db = *scoped
	}
	db.Statement.Settings.Store(dataScopeAppliedSetting, true)
}

func lookupDataScopeDirective(ctx context.Context) (dataScopeDirective, bool) {
	if ctx == nil {
		return dataScopeDirective{}, false
	}
	directive, ok := ctx.Value(dataScopeDirectiveContextKey{}).(dataScopeDirective)
	return directive, ok
}

// Only plain Model(&T{}) queries are auto-scoped in phase 1; aliased/custom/joined shapes stay manual.
func isSimpleDataScopeTarget(stmt *gorm.Statement) bool {
	if stmt == nil || stmt.Schema == nil {
		return false
	}
	if stmt.TableExpr != nil {
		return false
	}
	if len(stmt.Joins) > 0 {
		return false
	}
	return stmt.Table == "" || stmt.Table == stmt.Schema.Table
}
