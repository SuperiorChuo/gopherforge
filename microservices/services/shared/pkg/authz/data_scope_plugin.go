package authz

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

type dataScopeDirectiveContextKey struct{}

type dataScopeDirective struct {
	Disabled bool
	Scope    UserDataScope
}

// ScopedModelKind 描述受管模型的数据范围形态（authz 收敛批次 1 注册表化：
// 原实现硬编码各服务 localmodel 类型，shared 无法引用服务私有 model 包，
// 改为服务侧启动时注册，天然支持各服务模型集差异）。
type ScopedModelKind int

const (
	// ScopeByUserEntity 按用户实体过滤（id、department_id 两列，用户表形态）。
	ScopeByUserEntity ScopedModelKind = iota
	// ScopeByOwner 按归属人过滤（user_id 列，文件/日志/操作记录形态）。
	ScopeByOwner
)

var (
	userModelType = reflect.TypeOf(model.User{})

	scopedModelMu    sync.RWMutex
	scopedModelKinds = map[reflect.Type]ScopedModelKind{}
)

// RegisterScopedModel 注册受管模型的数据范围形态，供 DataScopePlugin 自动过滤。
// 服务启动装配时调用（重复注册同一类型以后者为准，幂等）。
func RegisterScopedModel(modelType reflect.Type, kind ScopedModelKind) {
	if modelType == nil {
		return
	}
	scopedModelMu.Lock()
	defer scopedModelMu.Unlock()
	scopedModelKinds[modelType] = kind
}

// lookupScopedModelKind 返回已注册模型的范围形态；未注册返回 (0, false)。
func lookupScopedModelKind(modelType reflect.Type) (ScopedModelKind, bool) {
	scopedModelMu.RLock()
	defer scopedModelMu.RUnlock()
	kind, ok := scopedModelKinds[modelType]
	return kind, ok
}

const dataScopeAppliedSetting = "go_admin_kit:data_scope_applied"

// EnableDataScope 标记查询上下文，启用由插件管理的数据范围过滤。
func EnableDataScope(ctx context.Context, scope UserDataScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, dataScopeDirectiveContextKey{}, dataScopeDirective{
		Scope: scope,
	})
}

// DisableDataScope 显式禁用由插件管理的数据范围过滤。
func DisableDataScope(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, dataScopeDirectiveContextKey{}, dataScopeDirective{
		Disabled: true,
	})
}

// ForceSelfScope 强制由插件管理的查询对指定的用户 ID 使用本人范围。
func ForceSelfScope(ctx context.Context, userID uint) context.Context {
	return EnableDataScope(ctx, UserDataScope{
		Scope:  DataScopeSelf,
		UserID: userID,
	})
}

// DataScopePlugin 对支持的模型应用可选的数据范围过滤。
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
	default:
		kind, ok := lookupScopedModelKind(db.Statement.Schema.ModelType)
		if !ok {
			return
		}
		switch kind {
		case ScopeByOwner:
			scoped = ApplyOwnerScope(db, directive.Scope, "user_id")
		case ScopeByUserEntity:
			scoped = ApplyUserEntityScope(db, directive.Scope, "id", "department_id")
		default:
			return
		}
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

// 阶段一仅对简单的 Model(&T{}) 查询自动应用数据范围；别名/自定义/联表等形态仍需手动处理。
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
