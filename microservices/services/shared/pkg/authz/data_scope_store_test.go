package authz

import (
	"context"
	"slices"
	"testing"

	model "github.com/go-admin-kit/services/shared/pkg/model"
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
