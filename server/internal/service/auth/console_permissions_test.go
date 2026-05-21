package auth

import (
	"context"
	"slices"
	"testing"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
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
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	user := &model.User{
		Roles: []model.Role{{Code: "super_admin"}},
	}
	got := ConsolePermissionsForUser(context.Background(), user, []string{
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
