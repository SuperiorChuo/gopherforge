package authz

import (
	"context"
	"fmt"
	"slices"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

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
	if store := currentPersistence().DataScope; store != nil {
		return store
	}
	return databaseDataScopeStore{}
}

func (r *DataScopeResolver) departmentTreeCache() DepartmentTreeCache {
	if r != nil && r.cache != nil {
		return r.cache
	}
	return defaultDepartmentTreeCache
}

func (s databaseDataScopeStore) ListDepartments(ctx context.Context) ([]model.Department, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	db := s.db
	if db == nil {
		db = currentDefaultDB()
	}
	if db == nil {
		return nil, fmt.Errorf("authz default db not configured")
	}
	var depts []model.Department
	//nolint:unbounded-find -- 数据权限解析需要当前租户的完整部门树并统一写入有界 TTL 缓存。
	if err := db.WithContext(ctx).Model(&model.Department{}).Select("id", "parent_id").Find(&depts).Error; err != nil {
		return nil, err
	}
	return depts, nil
}

func (s databaseDataScopeStore) ListRoleDataScopeDepartmentIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	db := s.db
	if db == nil {
		db = currentDefaultDB()
	}
	if db == nil {
		return nil, fmt.Errorf("authz default db not configured")
	}
	var relations []model.RoleDataScopeDepartment
	if err := db.WithContext(ctx).
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
