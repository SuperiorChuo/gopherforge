package authz

import (
	"context"
	"testing"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// resetDefaultDepartmentTreeCache 清空包级默认缓存（测试隔离）。
// 移植自 system/internal/pkg/authz/data_scope_cache_test.go。
func resetDefaultDepartmentTreeCache() {
	if cache, ok := NewDataScopeResolver(nil).departmentTreeCache().(*layeredDepartmentTreeCache); ok {
		cache.clearLocal(0)
	}
}

// withoutAuthzGlobals 清空包级全局装配（authz 收敛批次 1 注入化改造后，
// 原实现的 database.DB / redisstore.Client 全局置空改为 SetDefaultDB(nil) /
// SetRemoteCache(nil)，语义一致）。
func withoutAuthzGlobals(t *testing.T) {
	t.Helper()

	resetDefaultDepartmentTreeCache()
	oldDB := currentDefaultDB()
	oldCache := currentRemoteCache()
	SetDefaultDB(nil)
	SetRemoteCache(nil)
	restorePersistence := SetPersistence(Persistence{})
	t.Cleanup(func() {
		restorePersistence()
		resetDefaultDepartmentTreeCache()
		SetDefaultDB(oldDB)
		SetRemoteCache(oldCache)
	})
}

type stubDataScopeStore struct {
	departments         []model.Department
	departmentErr       error
	roleDepartmentIDs   []uint
	roleDepartmentErr   error
	departmentCalls     int
	roleDepartmentCalls int
	lastRoleIDs         []uint
}

func (s *stubDataScopeStore) ListDepartments(ctx context.Context) ([]model.Department, error) {
	s.departmentCalls++
	if s.departmentErr != nil {
		return nil, s.departmentErr
	}
	return append([]model.Department(nil), s.departments...), nil
}

func (s *stubDataScopeStore) ListRoleDataScopeDepartmentIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	s.roleDepartmentCalls++
	s.lastRoleIDs = append([]uint(nil), roleIDs...)
	if s.roleDepartmentErr != nil {
		return nil, s.roleDepartmentErr
	}
	return append([]uint(nil), s.roleDepartmentIDs...), nil
}

type stubDepartmentTreeCache struct {
	departments []model.Department
	getCalls    int
	setCalls    int
	invalidate  int
}

func (s *stubDepartmentTreeCache) GetDepartmentTree(ctx context.Context) ([]model.Department, bool) {
	s.getCalls++
	if s.departments == nil {
		return nil, false
	}
	return append([]model.Department(nil), s.departments...), true
}

func (s *stubDepartmentTreeCache) SetDepartmentTree(ctx context.Context, depts []model.Department) error {
	s.setCalls++
	s.departments = append([]model.Department(nil), depts...)
	return nil
}

func (s *stubDepartmentTreeCache) InvalidateDepartmentTree(ctx context.Context) error {
	s.invalidate++
	s.departments = nil
	return nil
}

type stubUserWithRolesStore struct {
	db *gorm.DB
}

func (s stubUserWithRolesStore) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	err := s.db.WithContext(ctx).Preload("Roles").First(&user, id).Error
	return &user, err
}
