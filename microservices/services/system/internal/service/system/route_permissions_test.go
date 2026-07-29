package system

// Drift guard between the menu seed and the frontend route guard.
//
// The console guard cannot derive "who to block" from /user/menus: that endpoint
// returns an already permission-filtered tree, so an absent menu is
// indistinguishable from an unauthorized one. The frontend therefore keeps a
// hand-written ROUTE_PERMISSIONS table, and this test keeps it honest — every
// seeded leaf that carries a permission code must be guarded or explicitly
// exempted with a reason.
//
// Skips when the web sources are absent (per-service Docker build contexts do
// not include them), so it only runs in the monorepo workspace.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const webRoutePermissionsPath = "../../../../../web/src/router/route-permissions.ts"

// parseTSStringMap pulls 'key': 'value' pairs out of one exported TS const.
func parseTSStringMap(t *testing.T, source, constName string) map[string]string {
	t.Helper()

	start := strings.Index(source, "export const "+constName)
	if start < 0 {
		t.Fatalf("const %s not found in route-permissions.ts", constName)
	}
	body := source[start:]
	if end := strings.Index(body, "\n}"); end >= 0 {
		body = body[:end]
	}

	pairs := regexp.MustCompile(`'([^']+)':\s*'([^']*)'`).FindAllStringSubmatch(body, -1)
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		out[pair[1]] = pair[2]
	}
	return out
}

func TestRoutePermissionsCoverSeededMenuPermissions(t *testing.T) {
	raw, err := os.ReadFile(webRoutePermissionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("web sources unavailable (per-service build context); skipping drift guard")
		}
		t.Fatalf("read route-permissions.ts: %v", err)
	}
	source := string(raw)

	guarded := parseTSStringMap(t, source, "ROUTE_PERMISSIONS")
	exempt := parseTSStringMap(t, source, "ROUTE_PERMISSION_EXEMPT")
	if len(guarded) == 0 {
		t.Fatal("parsed zero entries from ROUTE_PERMISSIONS; parser or file shape changed")
	}

	for _, menu := range defaultMenuSeed {
		// Containers (Component == "Layout") are not routable pages.
		if menu.Permission == "" || menu.Component == "" || menu.Component == "Layout" {
			continue
		}
		if _, ok := exempt[menu.Path]; ok {
			continue
		}

		code, ok := guarded[menu.Path]
		if !ok {
			t.Errorf("seeded menu %q (permission %q) has no route guard entry; add it to ROUTE_PERMISSIONS or ROUTE_PERMISSION_EXEMPT in %s",
				menu.Path, menu.Permission, filepath.Base(webRoutePermissionsPath))
			continue
		}
		if code != menu.Permission {
			t.Errorf("route guard for %q uses %q but the menu seed requires %q", menu.Path, code, menu.Permission)
		}
	}
}
