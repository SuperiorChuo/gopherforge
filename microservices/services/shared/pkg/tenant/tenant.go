// Package tenant provides multi-tenant context helpers and a GORM isolation
// plugin（多租户 M2 引入，cmd/main.go 注册）。对标 yudao 透明化封装后泛化
// （B 档①）：模型白名单（曾漏 Post）改为 schema 驱动——凡带 tenant_id 列的
// 模型自动隔离，DisableScope 为平台级跨租户操作的显式逃生口。手写 scope
// 保留为第一道，插件是防漏挂的第二道网（双重过滤无害）。
//
// 已知边界：db.Raw / Exec 原生 SQL 不经回调，不受保护；Create 时 ctx 租户为
// 权威值（显式跨租户写入需 DisableScope）；无租户上下文（后台任务/系统操作/
// 启动路径）不加过滤，与既有 DAO 语义一致。
//
// 本包为共享租户上下文工具：identity / ai / audit / file / system 等服务的
// middleware 写、service/dao 经 tenant.FromContext 读，共享同一 typed key。
package tenant

import (
	"context"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/go-admin-kit/services/shared/pkg/tenantctx"
)

// contextKey 私有类型避免与 context 中其它 string key 冲突（SA1029）。
type contextKey string

// ContextKey is the request context key for the active tenant id.
//
// authz 收敛批次 1：统一委托 shared/pkg/tenantctx 的 Key（typed key 与
// monitor 原 tenantctx 体系合并），各服务 auth middleware 的
// TenantIDContextKey 别名自动落到同一个 key，消除租户上下文分叉。
const ContextKey = tenantctx.Key

// PlatformAdminContextKey 平台管理员标记（SA1029 typed key）。auth 中间件写入，
// 各服务 dao/service 读取；放本 leaf 包避免 middleware↔dao 循环依赖。
const PlatformAdminContextKey contextKey = "platform_admin"

// DefaultID is the platform default tenant used when context has no tenant.
const DefaultID uint = 1

type disableScopeKey struct{}

// FromContext returns tenant id from context (0 if absent).
// 委托 tenantctx：全仓统一 key（authz 收敛批次 1）。
func FromContext(ctx context.Context) uint {
	return tenantctx.FromContext(ctx)
}

// WithContext stores tenant id on context.
// 委托 tenantctx：全仓统一 key（authz 收敛批次 1）。
func WithContext(ctx context.Context, tenantID uint) context.Context {
	return tenantctx.WithContext(ctx, tenantID)
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

// Normalize maps 0 → DefaultID.
func Normalize(id uint) uint {
	if id == 0 {
		return DefaultID
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

// FromContextOrDefault returns tenant from context, or DefaultID when missing.
func FromContextOrDefault(ctx context.Context) uint {
	return Normalize(FromContext(ctx))
}

// IDFromContext returns the active tenant id, defaulting to DefaultID
// (file-service 历史命名，等价于 FromContextOrDefault).
func IDFromContext(ctx context.Context) uint {
	return FromContextOrDefault(ctx)
}

// EnsureID keeps a positive existing tenant id; otherwise resolves from context
// (defaulting to DefaultID). Used on create/write paths so async or event-driven
// writers can stamp TenantID before context is lost.
func EnsureID(ctx context.Context, existing uint) uint {
	if existing > 0 {
		return existing
	}
	return FromContextOrDefault(ctx)
}

// ApplyFilter constrains a query to the actor tenant in ctx (default tenant 1).
// 手写第一道过滤；与插件叠加时双重过滤无害。
func ApplyFilter(query *gorm.DB, ctx context.Context) *gorm.DB {
	if query == nil {
		return query
	}
	return query.Where("tenant_id = ?", FromContextOrDefault(ctx))
}

const tenantAppliedSetting = "go_admin_kit:tenant_scope_applied"

// Plugin applies tenant_id filters for every model whose schema carries a
// tenant_id column (schema-driven; platform-level tables without the column
// are naturally exempt).
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
	if !ensureSchema(db) || !isTenantSchema(db) {
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
	if !ensureSchema(db) || !isTenantSchema(db) {
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
	// 仅在语句已有 WHERE 时追加——空条件的全局更新/删除仍交给 gorm 的
	// ErrMissingWhereClause 保护，避免"无条件更新"静默降级成"整租户更新"。
	if db == nil || db.Statement == nil {
		return
	}
	if _, hasWhere := db.Statement.Clauses["WHERE"]; !hasWhere {
		return
	}
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

// isTenantSchema reports whether the parsed schema carries a tenant_id column.
func isTenantSchema(db *gorm.DB) bool {
	sc := db.Statement.Schema
	if sc == nil {
		return false
	}
	field := sc.LookUpField("TenantID")
	return field != nil && field.DBName == "tenant_id"
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
