package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
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
	departmentTreeCacheKey = "authz:department_tree"
	departmentTreeCacheTTL = 5 * time.Minute
)

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

type databaseDataScopeStore struct{}

type redisDepartmentTreeCache struct{}

// UserDataScope is a reusable data permission result for business queries.
type UserDataScope struct {
	Scope         DataScope
	UserID        uint
	DepartmentID  uint
	DepartmentIDs []uint
	RoleIDs       []uint
	RoleCodes     []string
}

// ResolveUserDataScope resolves the base data permission scope from a user and roles.
//
// Role data_scope is the primary configuration; legacy role codes are compatibility fallbacks:
// super_admin/admin always get all data, and dept_admin gets department-tree data when data_scope is unset.
func ResolveUserDataScope(user *model.User) UserDataScope {
	scope, err := ResolveUserDataScopeContext(context.Background(), user)
	if err != nil {
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
	return scope
}

func ResolveUserDataScopeContext(ctx context.Context, user *model.User) (UserDataScope, error) {
	return NewDataScopeResolver(nil).ResolveUserDataScopeContext(ctx, user)
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

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	userDAO := auth.UserDAO{}
	user, err := userDAO.GetUserWithRolesContext(ctx, uid)
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
		return query.Where(userColumn+" IN (?)",
			query.Session(&gorm.Session{NewDB: true}).
				Model(&model.User{}).
				Select("id").
				Where("department_id IN ?", scope.DepartmentIDs),
		)
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

func departmentIDs(departmentID uint) []uint {
	if departmentID == 0 {
		return nil
	}
	return []uint{departmentID}
}

func resolveDepartmentTreeIDs(departmentID uint) []uint {
	ids, err := resolveDepartmentTreeIDsContext(context.Background(), departmentID)
	if err != nil {
		return departmentIDs(departmentID)
	}
	return ids
}

func resolveDepartmentTreeIDsContext(ctx context.Context, departmentID uint) ([]uint, error) {
	return NewDataScopeResolver(nil).resolveDepartmentTreeIDsContext(ctx, departmentID)
}

func (r *DataScopeResolver) resolveDepartmentTreeIDsContext(ctx context.Context, departmentID uint) ([]uint, error) {
	ids := departmentIDs(departmentID)
	if departmentID == 0 {
		return ids, nil
	}

	depts, err := r.loadDepartmentTreeContext(ctx)
	if err != nil {
		return nil, err
	}

	collectChildDepartmentIDs(depts, departmentID, &ids)
	return ids, nil
}

func (r *DataScopeResolver) loadDepartmentTreeContext(ctx context.Context) ([]model.Department, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cache := r.departmentTreeCache()
	if depts, ok := cache.GetDepartmentTree(ctx); ok {
		return depts, nil
	}

	depts, err := r.dataScopeStore().ListDepartments(ctx)
	if err != nil {
		return nil, err
	}

	_ = cache.SetDepartmentTree(ctx, depts)
	return depts, nil
}

func (redisDepartmentTreeCache) GetDepartmentTree(ctx context.Context) ([]model.Department, bool) {
	if redisstore.Client == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := redisstore.Client.Get(ctx, departmentTreeCacheKey).Bytes()
	if err != nil {
		return nil, false
	}

	var rows []departmentTreeCacheRow
	if err := json.Unmarshal(data, &rows); err != nil {
		_ = redisDepartmentTreeCache{}.InvalidateDepartmentTree(ctx)
		return nil, false
	}

	return departmentRowsToModels(rows), true
}

func (redisDepartmentTreeCache) SetDepartmentTree(ctx context.Context, depts []model.Department) error {
	if redisstore.Client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rows := make([]departmentTreeCacheRow, 0, len(depts))
	for _, dept := range depts {
		rows = append(rows, departmentTreeCacheRow{
			ID:       dept.ID,
			ParentID: dept.ParentID,
		})
	}

	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return redisstore.Client.Set(ctx, departmentTreeCacheKey, data, departmentTreeCacheTTL).Err()
}

func departmentRowsToModels(rows []departmentTreeCacheRow) []model.Department {
	depts := make([]model.Department, 0, len(rows))
	for _, row := range rows {
		depts = append(depts, model.Department{
			ID:       row.ID,
			ParentID: row.ParentID,
		})
	}
	return depts
}

func InvalidateDepartmentTreeCache() error {
	return redisDepartmentTreeCache{}.InvalidateDepartmentTree(context.Background())
}

func (redisDepartmentTreeCache) InvalidateDepartmentTree(ctx context.Context) error {
	if redisstore.Client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return redisstore.Client.Del(ctx, departmentTreeCacheKey).Err()
}

func resolveRoleDataScope(role model.Role) DataScope {
	if role.Code == "super_admin" || role.Code == "admin" {
		return DataScopeAll
	}

	if dataScope, ok := normalizeDataScope(role.DataScope); ok {
		return dataScope
	}

	switch role.Code {
	case "dept_admin":
		return DataScopeDepartmentTree
	default:
		return DataScopeSelf
	}
}

func normalizeDataScope(value string) (DataScope, bool) {
	switch DataScope(value) {
	case DataScopeAll, DataScopeDepartment, DataScopeDepartmentTree, DataScopeSelf, DataScopeCustom, DataScopeNone:
		return DataScope(value), true
	default:
		return "", false
	}
}

func maxDataScope(current, candidate DataScope) DataScope {
	if dataScopeRank(candidate) > dataScopeRank(current) {
		return candidate
	}
	return current
}

func dataScopeRank(scope DataScope) int {
	switch scope {
	case DataScopeAll:
		return 5
	case DataScopeDepartmentTree:
		return 4
	case DataScopeCustom:
		return 3
	case DataScopeDepartment:
		return 2
	case DataScopeSelf:
		return 1
	default:
		return 0
	}
}

func roleDataScopeDepartmentIDs(role model.Role) []uint {
	ids := append([]uint(nil), role.DataScopeDepartmentIDs...)
	for _, relation := range role.DataScopeDepartments {
		ids = append(ids, relation.DepartmentID)
	}
	return uniqueUintIDs(ids)
}

func (r *DataScopeResolver) loadRoleDataScopeDepartmentIDsContext(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	roleIDs = uniqueUintIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil, nil
	}

	ids, err := r.dataScopeStore().ListRoleDataScopeDepartmentIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	return uniqueUintIDs(ids), nil
}

func (r *DataScopeResolver) dataScopeStore() DataScopeStore {
	if r != nil && r.store != nil {
		return r.store
	}
	return databaseDataScopeStore{}
}

func (r *DataScopeResolver) departmentTreeCache() DepartmentTreeCache {
	if r != nil && r.cache != nil {
		return r.cache
	}
	return redisDepartmentTreeCache{}
}

func (databaseDataScopeStore) ListDepartments(ctx context.Context) ([]model.Department, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var depts []model.Department
	if err := database.DB.WithContext(ctx).Model(&model.Department{}).Select("id", "parent_id").Find(&depts).Error; err != nil {
		return nil, err
	}
	return depts, nil
}

func (databaseDataScopeStore) ListRoleDataScopeDepartmentIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var relations []model.RoleDataScopeDepartment
	if err := database.DB.WithContext(ctx).
		Model(&model.RoleDataScopeDepartment{}).
		Select("department_id").
		Where("role_id IN ?", roleIDs).
		Find(&relations).Error; err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(relations))
	for _, relation := range relations {
		ids = append(ids, relation.DepartmentID)
	}
	return uniqueUintIDs(ids), nil
}

func uniqueUintIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}

	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	slices.Sort(unique)
	return unique
}

func collectChildDepartmentIDs(depts []model.Department, parentID uint, ids *[]uint) {
	for _, dept := range depts {
		if dept.ParentID == parentID {
			*ids = append(*ids, dept.ID)
			collectChildDepartmentIDs(depts, dept.ID, ids)
		}
	}
}
