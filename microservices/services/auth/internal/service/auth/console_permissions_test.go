package auth

import (
	"context"
	"slices"
	"testing"

	"github.com/go-admin-kit/services/auth/internal/model"
)

func TestConsoleRoleCodesTrimsDeduplicatesAndSorts(t *testing.T) {
	got := ConsoleRoleCodes([]model.Role{
		{Code: " super_admin "},
		{Code: ""},
		{Code: "operator"},
		{Code: "super_admin"},
	})
	want := []string{"operator", "super_admin"}
	if !slices.Equal(got, want) {
		t.Fatalf("ConsoleRoleCodes() = %#v, want %#v", got, want)
	}
}

func TestConsolePermissionsForUserAddsAliasesAndSuperAdminDefaults(t *testing.T) {
	user := &model.User{
		Roles: []model.Role{{Code: "super_admin"}},
	}
	// A zero-value route service falls back to the static route seed, so the
	// test needs no database.
	got := ConsolePermissionsForUser(context.Background(), ConsoleRouteService{}, user, []string{
		"system:user:list",
		"system:role:update",
	})

	for _, permission := range []string{
		"system:user:list",
		"system:role:update",
		"rbac.read",
		"rbac.write",
		"dashboard.view",
		"settings.write",
	} {
		if !slices.Contains(got, permission) {
			t.Fatalf("ConsolePermissionsForUser() missing %q in %#v", permission, got)
		}
	}
	if !slices.IsSorted(got) {
		t.Fatalf("ConsolePermissionsForUser() should return sorted permissions: %#v", got)
	}
}
