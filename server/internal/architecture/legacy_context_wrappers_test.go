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

var legacyContextWrapperDirs = []string{
	"dao",
	"service",
	"pkg",
	"middleware",
}

func TestLegacyContextWrappersAreDeprecated(t *testing.T) {
	var violations []string

	for _, dir := range legacyContextWrapperDirs {
		root := filepath.Join(internalDir, dir)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fileViolations, err := legacyContextWrapperViolations(path)
			if err != nil {
				return err
			}
			violations = append(violations, fileViolations...)
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s files: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("legacy non-Context wrappers that call context.Background must be documented as deprecated:\n%s", strings.Join(violations, "\n"))
	}
}

func legacyContextWrapperViolations(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	contextImports := contextImportNames(file)
	if len(contextImports) == 0 {
		return nil, nil
	}

	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !ast.IsExported(fn.Name.Name) || strings.HasSuffix(fn.Name.Name, "Context") {
			continue
		}
		if hasContextParameter(fn) {
			continue
		}

		successor := calledContextSuccessor(fn, contextImports)
		if successor == "" {
			continue
		}

		doc := ""
		if fn.Doc != nil {
			doc = fn.Doc.Text()
		}
		if strings.Contains(doc, "Deprecated:") && strings.Contains(doc, successor) {
			continue
		}

		position := fset.Position(fn.Pos())
		violations = append(violations, fmt.Sprintf("%s:%d: %s must include a Deprecated: doc comment pointing to %s", relativeInternalPath(path), position.Line, fn.Name.Name, successor))
	}

	return violations, nil
}

func contextImportNames(file *ast.File) map[string]struct{} {
	names := map[string]struct{}{}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "context" {
			continue
		}
		name := "context"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." || name == "_" {
			continue
		}
		names[name] = struct{}{}
	}
	return names
}

func hasContextParameter(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		if isContextType(field.Type) {
			return true
		}
	}
	return false
}

func isContextType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "context"
}

func calledContextSuccessor(fn *ast.FuncDecl, contextImports map[string]struct{}) string {
	var successor string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if successor != "" {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok || !callHasContextBackgroundArg(call, contextImports) {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if strings.HasSuffix(fun.Name, "Context") {
				successor = fun.Name
			}
		case *ast.SelectorExpr:
			if strings.HasSuffix(fun.Sel.Name, "Context") {
				successor = fun.Sel.Name
			}
		}
		return true
	})
	return successor
}

func callHasContextBackgroundArg(call *ast.CallExpr, contextImports map[string]struct{}) bool {
	for _, arg := range call.Args {
		if isContextBackgroundCall(arg, contextImports) {
			return true
		}
	}
	return false
}

func isContextBackgroundCall(expr ast.Expr, contextImports map[string]struct{}) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Background" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = contextImports[ident.Name]
	return ok
}
