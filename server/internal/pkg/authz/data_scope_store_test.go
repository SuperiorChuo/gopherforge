package authz

import (
	"context"
	"slices"
	"testing"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
)

func TestDataScopeResolverUsesInjectedStoreForDepartmentTree(t *testing.T) {
	withoutAuthzGlobals(t)

	store := &stubDataScopeStore{
		departments: []model.Department{
			{ID: 11, ParentID: 10},
			{ID: 12, ParentID: 11},
		},
	}
	resolver := NewDataScopeResolver(store)

	got, err := resolver.ResolveUserDataScopeContext(context.Background(), &model.User{
		ID:           7,
		DepartmentID: 10,
		Roles: []model.Role{{
			ID:        3,
			Code:      "dept_admin",
			DataScope: string(DataScopeDepartmentTree),
		}},
	})
	if err != nil {
		t.Fatalf("ResolveUserDataScopeContext() error = %v", err)
	}
	if !slices.Equal(got.DepartmentIDs, []uint{10, 11, 12}) {
		t.Fatalf("department ids = %#v, want [10 11 12]", got.DepartmentIDs)
	}
	if store.departmentCalls != 1 {
		t.Fatalf("department store calls = %d, want 1", store.departmentCalls)
	}
	if store.roleDepartmentCalls != 0 {
		t.Fatalf("role department store calls = %d, want 0", store.roleDepartmentCalls)
	}
}

func TestDataScopeResolverUsesInjectedStoreForCustomDepartmentFallback(t *testing.T) {
	withoutAuthzGlobals(t)

	store := &stubDataScopeStore{
		roleDepartmentIDs: []uint{21, 20, 20},
	}
	resolver := NewDataScopeResolver(store)

	got, err := resolver.ResolveUserDataScopeContext(context.Background(), &model.User{
		ID:           8,
		DepartmentID: 10,
		Roles: []model.Role{{
			ID:        5,
			Code:      "regional_admin",
			DataScope: string(DataScopeCustom),
		}},
	})
	if err != nil {
		t.Fatalf("ResolveUserDataScopeContext() error = %v", err)
	}
	if !slices.Equal(got.DepartmentIDs, []uint{20, 21}) {
		t.Fatalf("department ids = %#v, want [20 21]", got.DepartmentIDs)
	}
	if !slices.Equal(store.lastRoleIDs, []uint{5}) {
		t.Fatalf("role ids passed to store = %#v, want [5]", store.lastRoleIDs)
	}
	if store.departmentCalls != 0 {
		t.Fatalf("department store calls = %d, want 0", store.departmentCalls)
	}
	if store.roleDepartmentCalls != 1 {
		t.Fatalf("role department store calls = %d, want 1", store.roleDepartmentCalls)
	}
}

func TestDataScopeResolverUsesInjectedDepartmentTreeCache(t *testing.T) {
	withoutAuthzGlobals(t)

	store := &stubDataScopeStore{
		departments: []model.Department{
			{ID: 11, ParentID: 10},
			{ID: 12, ParentID: 11},
		},
	}
	cache := &stubDepartmentTreeCache{}
	resolver := NewDataScopeResolverWithCache(store, cache)
	user := &model.User{
		ID:           7,
		DepartmentID: 10,
		Roles: []model.Role{{
			ID:        3,
			Code:      "dept_admin",
			DataScope: string(DataScopeDepartmentTree),
		}},
	}

	first, err := resolver.ResolveUserDataScopeContext(context.Background(), user)
	if err != nil {
		t.Fatalf("first ResolveUserDataScopeContext() error = %v", err)
	}
	second, err := resolver.ResolveUserDataScopeContext(context.Background(), user)
	if err != nil {
		t.Fatalf("second ResolveUserDataScopeContext() error = %v", err)
	}
	if !slices.Equal(first.DepartmentIDs, []uint{10, 11, 12}) {
		t.Fatalf("first department ids = %#v, want [10 11 12]", first.DepartmentIDs)
	}
	if !slices.Equal(second.DepartmentIDs, []uint{10, 11, 12}) {
		t.Fatalf("second department ids = %#v, want [10 11 12]", second.DepartmentIDs)
	}
	if store.departmentCalls != 1 {
		t.Fatalf("department store calls = %d, want 1", store.departmentCalls)
	}
	if cache.getCalls != 2 {
		t.Fatalf("cache get calls = %d, want 2", cache.getCalls)
	}
	if cache.setCalls != 1 {
		t.Fatalf("cache set calls = %d, want 1", cache.setCalls)
	}
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

func withoutAuthzGlobals(t *testing.T) {
	t.Helper()

	resetDefaultDepartmentTreeCache()
	oldDB := database.DB
	oldRedis := redisstore.Client
	database.DB = nil
	redisstore.Client = nil
	t.Cleanup(func() {
		resetDefaultDepartmentTreeCache()
		database.DB = oldDB
		redisstore.Client = oldRedis
	})
}
