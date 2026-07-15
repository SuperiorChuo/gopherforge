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

func TestProductionAPIHandlersDoNotReturnPlaceholderResponses(t *testing.T) {
	apiDir := filepath.Join(internalDir, "api")
	var violations []string

	err := filepath.WalkDir(apiDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileViolations, err := placeholderResponseReferences(path)
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
		t.Fatalf("production API handlers must not ship placeholder responses:\n%s", strings.Join(violations, "\n"))
	}
}

func placeholderResponseReferences(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if n.Sel.Name == "StatusNotImplemented" {
				violations = append(violations, placeholderViolation(fset, path, n.Pos(), "http.StatusNotImplemented"))
			}
		case *ast.BasicLit:
			if strings.Contains(strings.ToLower(n.Value), "not implemented") {
				violations = append(violations, placeholderViolation(fset, path, n.Pos(), "not implemented"))
			}
		}
		return true
	})

	return violations, nil
}

func placeholderViolation(fset *token.FileSet, path string, pos token.Pos, match string) string {
	position := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s", relativeInternalPath(path), position.Line, match)
}
