package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// bareAPIConstructorPattern matches zero-value API constructors such as
// NewUserAPI. Injected variants (NewUserAPIWithServices) do not match because
// they carry a suffix after "API".
var bareAPIConstructorPattern = regexp.MustCompile(`^New\w*API$`)

// allowedBareAPIConstructorCalls lists the bare (zero-value) API constructor
// calls that route composition may still perform. Each entry is either the
// documented legacy fallback branch taken when no dependencies are injected,
// or an API without injectable infrastructure. New route registrations must
// use the *WithService/*WithServices constructors fed from
// shared.Dependencies instead of growing this list.
var allowedBareAPIConstructorCalls = map[string]int{
	// api/routes.go newSystemAPIs: legacy zero-value branch (deps.DB == nil).
	"api/routes.go|newSystemAPIs|NewAuditLogAPI":             1,
	"api/routes.go|newSystemAPIs|NewDepartmentAPI":           1,
	"api/routes.go|newSystemAPIs|NewDictAPI":                 1,
	"api/routes.go|newSystemAPIs|NewFileAPI":                 1,
	"api/routes.go|newSystemAPIs|NewLoginLogAPI":             1,
	"api/routes.go|newSystemAPIs|NewMenuManagementAPI":       1,
	"api/routes.go|newSystemAPIs|NewNoticeAPI":               1,
	"api/routes.go|newSystemAPIs|NewNotificationAPI":         1,
	"api/routes.go|newSystemAPIs|NewOnlineUserAPI":           1,
	"api/routes.go|newSystemAPIs|NewOperationLogAPI":         1,
	"api/routes.go|newSystemAPIs|NewPermissionManagementAPI": 1,
	"api/routes.go|newSystemAPIs|NewRoleManagementAPI":       1,
	"api/routes.go|newSystemAPIs|NewSettingAPI":              1,
	"api/routes.go|newSystemAPIs|NewUserManagementAPI":       1,

	// api/auth/routes.go: legacy zero-value branches (deps.DB == nil) plus
	// CaptchaAPI, which has no injectable infrastructure.
	"api/auth/routes.go|newMenuAPIFromDeps|NewMenuAPI":              1,
	"api/auth/routes.go|newOAuthAPIFromDeps|NewOAuthAPI":            1,
	"api/auth/routes.go|newUserAPIFromDeps|NewUserAPI":              1,
	"api/auth/routes.go|RegisterPublicRoutesWithDeps|NewCaptchaAPI": 1,

	// api/monitor/routes.go: ServerAPI reads host metrics only; JobAPI wraps
	// the singleton injected via InitJobService; MySQL/Redis bare calls are
	// the legacy fallbacks replaced when deps carry the matching handle.
	"api/monitor/routes.go|RegisterProtectedRoutesWithDeps|NewJobAPI":    1,
	"api/monitor/routes.go|RegisterProtectedRoutesWithDeps|NewMySQLAPI":  1,
	"api/monitor/routes.go|RegisterProtectedRoutesWithDeps|NewRedisAPI":  1,
	"api/monitor/routes.go|RegisterProtectedRoutesWithDeps|NewServerAPI": 1,

	// api/common/routes.go: health/IP endpoints still assemble zero-value
	// APIs pending dependency injection.
	"api/common/routes.go|RegisterPublicRoutes|NewHealthAPI": 1,
	"api/common/routes.go|RegisterPublicRoutes|NewIPInfoAPI": 1,
}

// TestRouteCompositionUsesInjectedAPIConstructors keeps the composition root
// honest: route registration must assemble APIs through dependency-injected
// constructors, and every remaining bare New*API call needs an explicit
// allowlist entry.
func TestRouteCompositionUsesInjectedAPIConstructors(t *testing.T) {
	apiDir := filepath.Join(internalDir, "api")
	allowances := cloneAllowances(allowedBareAPIConstructorCalls)
	var violations []string

	err := filepath.WalkDir(apiDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Base(path) != "routes.go" {
			return nil
		}

		fileViolations, err := bareAPIConstructorCalls(path, allowances)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		t.Fatalf("scan route files: %v", err)
	}

	violations = append(violations, unusedAllowances(allowances)...)
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("route composition must use dependency-injected API constructors; bare New*API calls need an explicit allowlist entry:\n%s", strings.Join(violations, "\n"))
	}
}

func bareAPIConstructorCalls(path string, allowances map[string]int) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			name := calledFunctionName(call)
			if !bareAPIConstructorPattern.MatchString(name) {
				return true
			}

			relPath := relativeInternalPath(path)
			allowKey := strings.Join([]string{relPath, fn.Name.Name, name}, "|")
			if allowances[allowKey] > 0 {
				allowances[allowKey]--
				return true
			}

			position := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: %s called in %s", relPath, position.Line, name, fn.Name.Name))
			return true
		})
	}

	return violations, nil
}

func calledFunctionName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}
