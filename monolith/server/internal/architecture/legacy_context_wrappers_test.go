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

var legacyContextWrapperForbiddenPrefixes = []string{
	"dao/",
	"dao/auth/",
	"dao/monitor/",
	"dao/system/",
	"service/auth/",
	"service/monitor/",
	"service/system/",
	"pkg/cache/",
	"pkg/captcha/",
	"pkg/ipinfo/",
	"pkg/jwt/",
	"pkg/upload/",
	"middleware/",
}

var legacyContextWrapperForbiddenKeys = map[string]struct{}{
	"pkg/authz/data_scope.go||ResolveUserDataScope|ResolveUserDataScopeContext":                   {},
	"pkg/authz/permissions.go||UserHasPermission|UserHasPermissionContext":                        {},
	"pkg/authz/data_scope.go||InvalidateDepartmentTreeCache|InvalidateDepartmentTreeCacheContext": {},
}

var allowedLegacyContextBridges = map[string]int{
	// Record bridges existing gin.Context call sites to RecordContext while
	// preserving the request context when Gin provides one.
	"service/system/audit_log.go|AuditLogService|Record|RecordContext": 1,
}

func TestLegacyContextWrappersAreDeprecated(t *testing.T) {
	var violations []string
	allowedBridges := cloneAllowances(allowedLegacyContextBridges)

	for _, dir := range legacyContextWrapperDirs {
		root := filepath.Join(internalDir, dir)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fileViolations, err := legacyContextWrapperViolations(path, allowedBridges)
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
	for key, remaining := range allowedBridges {
		if remaining > 0 {
			violations = append(violations, fmt.Sprintf("unused legacy context bridge allowlist entry: %s", key))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("legacy non-Context wrappers under cleaned prefixes are forbidden; other wrappers that call context.Background must be documented as deprecated:\n%s", strings.Join(violations, "\n"))
	}
}

func TestCalledContextSuccessorDetectsBackgroundAlias(t *testing.T) {
	source := `package fixture

import "context"

func (s *Service) Legacy() error {
	ctx := context.Background()
	return s.LegacyContext(ctx)
}
`

	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if parsed, ok := decl.(*ast.FuncDecl); ok && parsed.Name.Name == "Legacy" {
			fn = parsed
			break
		}
	}
	if fn == nil {
		t.Fatal("Legacy fixture function not found")
	}

	if successor := calledContextSuccessor(fn, contextImportNames(file)); successor != "LegacyContext" {
		t.Fatalf("calledContextSuccessor() = %q, want LegacyContext", successor)
	}
}

func TestCalledContextSuccessorDetectsPropagatedBackgroundAlias(t *testing.T) {
	source := `package fixture

import "context"

func (s *Service) Legacy() error {
	base := context.Background()
	ctx := base
	return s.LegacyContext(ctx)
}
`

	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if parsed, ok := decl.(*ast.FuncDecl); ok && parsed.Name.Name == "Legacy" {
			fn = parsed
			break
		}
	}
	if fn == nil {
		t.Fatal("Legacy fixture function not found")
	}

	if successor := calledContextSuccessor(fn, contextImportNames(file)); successor != "LegacyContext" {
		t.Fatalf("calledContextSuccessor() = %q, want LegacyContext", successor)
	}
}

func TestLegacyContextBridgeAllowlistMatchesReceiver(t *testing.T) {
	source := `package fixture

func (s *AuditLogService) Record() {}
func (s *OtherService) Record() {}
`

	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var auditRecord *ast.FuncDecl
	var otherRecord *ast.FuncDecl
	for _, decl := range file.Decls {
		parsed, ok := decl.(*ast.FuncDecl)
		if !ok || parsed.Name.Name != "Record" {
			continue
		}
		switch receiverTypeName(parsed) {
		case "AuditLogService":
			auditRecord = parsed
		case "OtherService":
			otherRecord = parsed
		}
	}
	if auditRecord == nil || otherRecord == nil {
		t.Fatal("fixture record methods not found")
	}

	path := filepath.Join(internalDir, "service", "system", "audit_log.go")
	if !legacyContextBridgeAllowed(path, auditRecord, "RecordContext", cloneAllowances(allowedLegacyContextBridges)) {
		t.Fatal("AuditLogService.Record bridge should be allowlisted")
	}
	if legacyContextBridgeAllowed(path, otherRecord, "RecordContext", cloneAllowances(allowedLegacyContextBridges)) {
		t.Fatal("OtherService.Record bridge should not be allowlisted")
	}
}

func legacyContextWrapperViolations(path string, allowedBridges map[string]int) ([]string, error) {
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

		position := fset.Position(fn.Pos())
		if legacyContextBridgeAllowed(path, fn, successor, allowedBridges) {
			continue
		}
		if legacyContextWrapperForbidden(path, fn, successor) {
			violations = append(violations, fmt.Sprintf("%s:%d: %s legacy non-Context wrapper is forbidden in cleaned packages; remove it and call %s", relativeInternalPath(path), position.Line, fn.Name.Name, successor))
			continue
		}

		doc := ""
		if fn.Doc != nil {
			doc = fn.Doc.Text()
		}
		if strings.Contains(doc, "Deprecated:") && strings.Contains(doc, successor) {
			continue
		}

		violations = append(violations, fmt.Sprintf("%s:%d: %s must include a Deprecated: doc comment pointing to %s", relativeInternalPath(path), position.Line, fn.Name.Name, successor))
	}

	return violations, nil
}

func legacyContextWrappersForbiddenIn(path string) bool {
	relPath := relativeInternalPath(path)
	for _, prefix := range legacyContextWrapperForbiddenPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

func legacyContextWrapperForbidden(path string, fn *ast.FuncDecl, successor string) bool {
	if legacyContextWrappersForbiddenIn(path) {
		return true
	}
	key := strings.Join([]string{relativeInternalPath(path), receiverTypeName(fn), fn.Name.Name, successor}, "|")
	_, ok := legacyContextWrapperForbiddenKeys[key]
	return ok
}

func legacyContextBridgeAllowed(path string, fn *ast.FuncDecl, successor string, allowedBridges map[string]int) bool {
	key := strings.Join([]string{relativeInternalPath(path), receiverTypeName(fn), fn.Name.Name, successor}, "|")
	if allowedBridges[key] <= 0 {
		return false
	}
	allowedBridges[key]--
	return true
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch expr := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
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
	contextBackgroundNames := contextBackgroundAliases(fn, contextImports)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if successor != "" {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok || !callHasBackgroundContextArg(call, contextImports, contextBackgroundNames) {
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

func contextBackgroundAliases(fn *ast.FuncDecl, contextImports map[string]struct{}) map[string]struct{} {
	names := map[string]struct{}{}
	changed := true
	for changed {
		changed = false
		collectContextBackgroundAliases(fn, contextImports, names, &changed)
	}
	return names
}

func collectContextBackgroundAliases(fn *ast.FuncDecl, contextImports map[string]struct{}, names map[string]struct{}, changed *bool) {
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.AssignStmt:
			for index, rhs := range stmt.Rhs {
				if !isBackgroundContextExpr(rhs, contextImports, names) || index >= len(stmt.Lhs) {
					continue
				}
				if ident, ok := stmt.Lhs[index].(*ast.Ident); ok && !contextBackgroundAliasExists(names, ident.Name) {
					names[ident.Name] = struct{}{}
					*changed = true
				}
			}
		case *ast.ValueSpec:
			for index, value := range stmt.Values {
				if !isBackgroundContextExpr(value, contextImports, names) || index >= len(stmt.Names) {
					continue
				}
				name := stmt.Names[index].Name
				if contextBackgroundAliasExists(names, name) {
					continue
				}
				names[name] = struct{}{}
				*changed = true
			}
		}
		return true
	})
}

func isBackgroundContextExpr(expr ast.Expr, contextImports map[string]struct{}, contextBackgroundNames map[string]struct{}) bool {
	if isContextBackgroundCall(expr, contextImports) {
		return true
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return contextBackgroundAliasExists(contextBackgroundNames, ident.Name)
}

func contextBackgroundAliasExists(contextBackgroundNames map[string]struct{}, name string) bool {
	_, ok := contextBackgroundNames[name]
	return ok
}

func callHasBackgroundContextArg(call *ast.CallExpr, contextImports map[string]struct{}, contextBackgroundNames map[string]struct{}) bool {
	for _, arg := range call.Args {
		if isBackgroundContextExpr(arg, contextImports, contextBackgroundNames) {
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
