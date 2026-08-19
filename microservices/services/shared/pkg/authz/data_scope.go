package authz

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// DataScope is a data permission scope.
type DataScope string

const (
	DataScopeAll            DataScope = "all"
	DataScopeDepartment     DataScope = "department"
	DataScopeDepartmentTree DataScope = "department_tree"
	DataScopeSelf           DataScope = "self"
	DataScopeCustom         DataScope = "custom"
	DataScopeNone           DataScope = "none"
)

const (
	// departmentTreeCacheKeyPrefix is completed with the tenant id. The cached rows
	// come from a tenant-filtered query (the gorm tenant plugin appends tenant_id),
	// so a single shared key would let whichever tenant populated it first dictate
	// every other tenant's department scope.
	departmentTreeCacheKeyPrefix         = "authz:department_tree:"
	departmentTreeCacheTTL               = 5 * time.Minute
	departmentTreeLocalCacheTTL          = 30 * time.Second
	departmentTreeInvalidateChannel      = "authz:department_tree:invalidate"
	departmentTreeInvalidatePayloadClear = "clear"
)

func departmentTreeCacheKey(tenantID uint) string {
	return fmt.Sprintf("%s%d", departmentTreeCacheKeyPrefix, tenantID)
}

type departmentTreeCacheRow struct {
	ID       uint `json:"id"`
	ParentID uint `json:"parent_id"`
}

// DataScopeStore loads data permission dependencies.
type DataScopeStore interface {
	ListDepartments(ctx context.Context) ([]model.Department, error)
	ListRoleDataScopeDepartmentIDs(ctx context.Context, roleIDs []uint) ([]uint, error)
}

// DepartmentTreeCache caches department tree rows for data-scope resolution.
type DepartmentTreeCache interface {
	GetDepartmentTree(ctx context.Context) ([]model.Department, bool)
	SetDepartmentTree(ctx context.Context, depts []model.Department) error
	InvalidateDepartmentTree(ctx context.Context) error
}

// RemoteCache 是部门树跨实例失效的远端缓存抽象（authz 收敛批次 1 引入）。
// 服务侧用 SetRemoteCache 注入各自 redis 客户端（shared/pkg/authz 提供
// NewGoRedisRemoteCache 通用适配器）；未装配时缓存降级为纯本地（与原
// redisstore.Client == nil 语义一致）。
type RemoteCache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	PublishString(ctx context.Context, channel, payload string) error
	// StartSubscriber 订阅 channel，返回的 io.Closer 停止订阅。
	StartSubscriber(ctx context.Context, channel string, handler func(context.Context, string)) (io.Closer, error)
}

var (
	remoteCacheMu sync.RWMutex
	remoteCache   RemoteCache
	defaultDB     *gorm.DB
)

// SetRemoteCache 安装部门树远端缓存（nil 时降级为纯本地缓存）。
func SetRemoteCache(c RemoteCache) {
	remoteCacheMu.Lock()
	defer remoteCacheMu.Unlock()
	remoteCache = c
}

// currentRemoteCache 返回已装配的远端缓存；未装配时返回 nil（调用方降级）。
func currentRemoteCache() RemoteCache {
	remoteCacheMu.RLock()
	defer remoteCacheMu.RUnlock()
	return remoteCache
}

// SetDefaultDB 安装 databaseDataScopeStore 零值兜底所用的默认数据库连接。
// 服务启动装配时调用（原实现直接引用各服务进程级 database.DB 全局）。
func SetDefaultDB(db *gorm.DB) {
	remoteCacheMu.Lock()
	defer remoteCacheMu.Unlock()
	defaultDB = db
}

// currentDefaultDB 返回装配的默认数据库连接（可能为 nil，调用方自行处理）。
func currentDefaultDB() *gorm.DB {
	remoteCacheMu.RLock()
	defer remoteCacheMu.RUnlock()
	return defaultDB
}

// DataScopeResolver resolves user data permissions with injectable persistence.
type DataScopeResolver struct {
	store DataScopeStore
	cache DepartmentTreeCache
}

// NewDataScopeResolver creates a resolver. A nil store uses the default database-backed store.
func NewDataScopeResolver(store DataScopeStore) *DataScopeResolver {
	return &DataScopeResolver{store: store}
}

// NewDataScopeResolverWithCache creates a resolver with injectable persistence and department tree cache.
func NewDataScopeResolverWithCache(store DataScopeStore, cache DepartmentTreeCache) *DataScopeResolver {
	return &DataScopeResolver{store: store, cache: cache}
}

type databaseDataScopeStore struct {
	db *gorm.DB
}

// NewDatabaseDataScopeStore builds the default gorm-backed DataScopeStore
// from an injected database handle.
func NewDatabaseDataScopeStore(db *gorm.DB) DataScopeStore {
	return databaseDataScopeStore{db: db}
}

type departmentTreeLocalEntry struct {
	rows      []departmentTreeCacheRow
	expiresAt time.Time
}

type layeredDepartmentTreeCache struct {
	mu       sync.RWMutex
	byTenant map[uint]departmentTreeLocalEntry
	localTTL time.Duration
}

var defaultDepartmentTreeCache = &layeredDepartmentTreeCache{
	localTTL: departmentTreeLocalCacheTTL,
}

// UserDataScope is a reusable data permission result for business queries.
type UserDataScope struct {
	Scope         DataScope
	UserID        uint
	DepartmentID  uint
	DepartmentIDs []uint
	RoleIDs       []uint
	RoleCodes     []string
}

func ResolveUserDataScopeContext(ctx context.Context, user *model.User) (UserDataScope, error) {
	return NewDataScopeResolver(nil).ResolveUserDataScopeContext(ctx, user)
}

// ResolveUserDataScopeFallbackContext resolves scope and falls back to self scope on dependency errors.
func ResolveUserDataScopeFallbackContext(ctx context.Context, user *model.User) UserDataScope {
	return NewDataScopeResolver(nil).ResolveUserDataScopeFallbackContext(ctx, user)
}

func (r *DataScopeResolver) ResolveUserDataScopeFallbackContext(ctx context.Context, user *model.User) UserDataScope {
	scope, err := r.ResolveUserDataScopeContext(ctx, user)
	if err == nil {
		return scope
	}
	if user == nil {
		return UserDataScope{Scope: DataScopeNone}
	}
	return UserDataScope{
		Scope:         DataScopeSelf,
		UserID:        user.ID,
		DepartmentID:  user.DepartmentID,
		DepartmentIDs: departmentIDs(user.DepartmentID),
	}
}

func (r *DataScopeResolver) ResolveUserDataScopeContext(ctx context.Context, user *model.User) (UserDataScope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if user == nil {
		return UserDataScope{Scope: DataScopeNone}, nil
	}

	scope := UserDataScope{
		Scope:        DataScopeNone,
		UserID:       user.ID,
		DepartmentID: user.DepartmentID,
		RoleIDs:      make([]uint, 0, len(user.Roles)),
		RoleCodes:    make([]string, 0, len(user.Roles)),
	}

	if len(user.Roles) == 0 {
		scope.Scope = DataScopeSelf
		scope.DepartmentIDs = departmentIDs(user.DepartmentID)
		return scope, nil
	}

	customRoleIDs := make([]uint, 0)
	departmentIDsByRole := make([]uint, 0)

	for _, role := range user.Roles {
		scope.RoleIDs = append(scope.RoleIDs, role.ID)
		scope.RoleCodes = append(scope.RoleCodes, role.Code)

		roleScope := resolveRoleDataScope(role)
		switch roleScope {
		case DataScopeAll:
			scope.Scope = DataScopeAll
			return scope, nil
		case DataScopeDepartmentTree:
			scope.Scope = maxDataScope(scope.Scope, roleScope)
			ids, err := r.resolveDepartmentTreeIDsContext(ctx, user.DepartmentID)
			if err != nil {
				return scope, err
			}
			departmentIDsByRole = append(departmentIDsByRole, ids...)
		case DataScopeDepartment:
			scope.Scope = maxDataScope(scope.Scope, roleScope)
			departmentIDsByRole = append(departmentIDsByRole, departmentIDs(user.DepartmentID)...)
		case DataScopeCustom:
			scope.Scope = maxDataScope(scope.Scope, roleScope)
			roleDepartmentIDs := roleDataScopeDepartmentIDs(role)
			if len(roleDepartmentIDs) == 0 {
				customRoleIDs = append(customRoleIDs, role.ID)
				continue
			}
			departmentIDsByRole = append(departmentIDsByRole, roleDepartmentIDs...)
		case DataScopeSelf:
			scope.Scope = maxDataScope(scope.Scope, roleScope)
		case DataScopeNone:
			scope.Scope = maxDataScope(scope.Scope, roleScope)
		}
	}

	if len(customRoleIDs) > 0 {
		ids, err := r.loadRoleDataScopeDepartmentIDsContext(ctx, customRoleIDs)
		if err != nil {
			return scope, err
		}
		departmentIDsByRole = append(departmentIDsByRole, ids...)
	}

	switch scope.Scope {
	case DataScopeDepartment, DataScopeDepartmentTree, DataScopeCustom:
		scope.DepartmentIDs = uniqueUintIDs(departmentIDsByRole)
	case DataScopeSelf:
		scope.DepartmentIDs = departmentIDs(user.DepartmentID)
	default:
		scope.DepartmentIDs = nil
	}

	return scope, nil
}

// CanAccessAll reports whether the resolved scope can access all data.
func (s UserDataScope) CanAccessAll() bool {
	return s.Scope == DataScopeAll
}

// RoleCodesContextKey is the Gin context key under which PermissionMiddleware
// memoizes the caller's role codes for the duration of one request.
const RoleCodesContextKey = "authz_role_codes"

// RoleCodesFromGinContext returns the role codes memoized for this request.
func RoleCodesFromGinContext(c *gin.Context) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(RoleCodesContextKey)
	if !exists {
		return nil, false
	}
	codes, ok := value.([]string)
	return codes, ok
}

// roleCodesGrantAllScope mirrors resolveRoleDataScope's unconditional grant.
func roleCodesGrantAllScope(codes []string) bool {
	for _, code := range codes {
		if code == "super_admin" || code == "admin" {
			return true
		}
	}
	return false
}

// ResolveUserDataScopeFromContext resolves data permissions for the current Gin user_id.
func ResolveUserDataScopeFromContext(c *gin.Context) (UserDataScope, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return UserDataScope{Scope: DataScopeNone}, fmt.Errorf("user not found in context")
	}

	uid, ok := userID.(uint)
	if !ok {
		return UserDataScope{Scope: DataScopeNone}, fmt.Errorf("invalid user id in context")
	}

	// resolveRoleDataScope grants DataScopeAll to super_admin/admin on the role
	// code alone — it never reads role.DataScope — and the caller returns as soon
	// as that lands. So for those callers the user+roles round trip below can only
	// restate what PermissionMiddleware already read from the role-code cache.
	//
	// This cannot widen access: the request reached this handler by passing
	// PermissionMiddleware on the strength of these very codes, so trusting them
	// again here adds no staleness window that was not already accepted upstream.
	// Routes without PermissionMiddleware memoize nothing and fall through.
	if codes, ok := RoleCodesFromGinContext(c); ok && roleCodesGrantAllScope(codes) {
		return UserDataScope{Scope: DataScopeAll, UserID: uid, RoleCodes: codes}, nil
	}

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	users := currentPersistence().Users
	if users == nil {
		return UserDataScope{Scope: DataScopeNone}, ErrPersistenceNotConfigured
	}
	user, err := users.GetUserWithRolesContext(ctx, uid)
	if err != nil {
		return UserDataScope{Scope: DataScopeNone}, err
	}

	return ResolveUserDataScopeContext(ctx, user)
}

// ApplyUserEntityScope appends data permission conditions to user table queries.
func ApplyUserEntityScope(query *gorm.DB, scope UserDataScope, idColumn, departmentColumn string) *gorm.DB {
	switch scope.Scope {
	case DataScopeAll:
		return query
	case DataScopeDepartment, DataScopeDepartmentTree, DataScopeCustom:
		if len(scope.DepartmentIDs) == 0 {
			return query.Where("1 = 0")
		}
		return query.Where(departmentColumn+" IN ?", scope.DepartmentIDs)
	case DataScopeSelf:
		if scope.UserID == 0 {
			return query.Where("1 = 0")
		}
		return query.Where(idColumn+" = ?", scope.UserID)
	default:
		return query.Where("1 = 0")
	}
}

// ApplyOwnerScope appends data permission conditions to business tables with a user_id owner column.
func ApplyOwnerScope(query *gorm.DB, scope UserDataScope, userColumn string) *gorm.DB {
	switch scope.Scope {
	case DataScopeAll:
		return query
	case DataScopeDepartment, DataScopeDepartmentTree, DataScopeCustom:
		if len(scope.DepartmentIDs) == 0 {
			return query.Where("1 = 0")
		}
		return query.Where(userColumn+" IN (SELECT id FROM users WHERE department_id IN ?)", scope.DepartmentIDs)
	case DataScopeSelf:
		if scope.UserID == 0 {
			return query.Where("1 = 0")
		}
		return query.Where(userColumn+" = ?", scope.UserID)
	default:
		return query.Where("1 = 0")
	}
}

// ApplyUnownedResourceScope is for resources without persisted user_id or department_id ownership columns.
// Switch to ApplyOwnerScope or ApplyUserEntityScope after resource tables gain ownership columns.
func ApplyUnownedResourceScope(query *gorm.DB, scope UserDataScope) *gorm.DB {
	if scope.CanAccessAll() {
		return query
	}
	return query.Where("1 = 0")
}
