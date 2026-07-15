package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const internalDir = ".."

var guardedGlobalPackages = map[string]map[string]string{
	"github.com/go-admin-kit/server/internal/pkg/database": {
		"DB": "database.DB",
	},
	"github.com/go-admin-kit/server/internal/pkg/redis": {
		"Client": "redis.Client",
	},
}

var allowedAPIGlobalReferences = map[string]int{
	"api/common/health.go|databaseStatusClient|database.DB": 2,
	"api/common/health.go|redisPingClient|redis.Client":     2,
}

func TestAPILayerDoesNotUseGlobalDatabaseOrRedisClients(t *testing.T) {
	apiDir := filepath.Join(internalDir, "api")
	allowances := cloneAllowances(allowedAPIGlobalReferences)
	var violations []string

	err := filepath.WalkDir(apiDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileViolations, err := globalClientReferences(path, allowances)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		t.Fatalf("scan API files: %v", err)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("API handlers must not directly use global DB/Redis clients; use injected service/DAO dependencies instead:\n%s", strings.Join(violations, "\n"))
	}
}

func globalClientReferences(path string, allowances map[string]int) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	guardedImports := map[string]map[string]string{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		selectors, ok := guardedGlobalPackages[importPath]
		if !ok {
			continue
		}

		name := filepath.Base(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." || name == "_" {
			continue
		}
		guardedImports[name] = selectors
	}

	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}

			selectors, ok := guardedImports[ident.Name]
			if !ok {
				return true
			}

			canonical, ok := selectors[selector.Sel.Name]
			if !ok {
				return true
			}

			position := fset.Position(selector.Pos())
			relPath := relativeInternalPath(path)
			allowKey := strings.Join([]string{relPath, fn.Name.Name, canonical}, "|")
			if allowances[allowKey] > 0 {
				allowances[allowKey]--
				return true
			}

			violations = append(violations, fmt.Sprintf("%s:%d: %s (source: %s.%s)", relPath, position.Line, canonical, ident.Name, selector.Sel.Name))
			return true
		})
	}

	return violations, nil
}

func cloneAllowances(source map[string]int) map[string]int {
	clone := make(map[string]int, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func relativeInternalPath(path string) string {
	rel, err := filepath.Rel(internalDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
