package system

// 菜单种子与前端路由守卫之间的漂移守卫。
//
// 控制台守卫无法从 /user/menus 推导"应阻止谁"：该端点返回的已是按权限
// 过滤后的树，因此某个菜单缺失与未授权是无法区分的。前端因此维护了一份
// 手写的 ROUTE_PERMISSIONS 表，本测试确保它与种子保持一致——每个带有
// 权限码的种子叶子都必须被守卫，或带理由显式豁免。
//
// 当 web 源码缺失时跳过（按服务构建的 Docker 上下文不包含它们），
// 因此只在 monorepo 工作区中运行。

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const webRoutePermissionsPath = "../../../../../web/src/router/route-permissions.ts"

// parseTSStringMap 从某个导出的 TS const 中提取 'key': 'value' 对。
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
		// 容器（Component == "Layout"）不是可路由的页面。
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
