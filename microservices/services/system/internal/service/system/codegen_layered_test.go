package system

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

func TestCodegenGenerateTargetsSystemServiceLayers(t *testing.T) {
	files, err := NewCodegenServiceWithDB(newCodegenTestDB(t)).Generate(assetPlanRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	byPath := filesByPath(t, files)
	want := map[string]string{
		"microservices/services/system/internal/model/assets.go":              "package localmodel",
		"microservices/services/system/internal/dao/system/assets.go":         "package system",
		"microservices/services/system/internal/service/system/assets.go":     "package system",
		"microservices/services/system/internal/api/system/assets.go":         "package system",
		"microservices/web/src/api/assets.ts":                                 "import request from '@/utils/request'",
		"microservices/web/src/pages/system/assets/index.tsx":                 "export default function AssetsPage()",
		"microservices/services/monitor/migrations/000000_codegen_assets.sql": "system:assets:list",
	}
	for path, marker := range want {
		content, ok := byPath[path]
		if !ok {
			t.Fatalf("generated path %q missing; got %v", path, sortedFilePaths(files))
		}
		if !strings.Contains(content, marker) {
			t.Fatalf("%s missing %q", path, marker)
		}
	}

	for path, content := range byPath {
		if strings.HasSuffix(path, ".go") {
			if _, err := parser.ParseFile(token.NewFileSet(), path, content, parser.AllErrors); err != nil {
				t.Fatalf("parse %s: %v\n%s", path, err, content)
			}
		}
		for _, forbidden := range []string{"[[", "]]", "TODO", "手工接线", "manual wiring"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains unresolved marker %q", path, forbidden)
			}
		}
	}
}

func TestCodegenTenantFieldIsServerManaged(t *testing.T) {
	files, err := NewCodegenServiceWithDB(newCodegenSchemaDB(t)).Generate(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{
			{Name: "name", InSearch: true, InForm: true},
			{Name: "tenant_id", InForm: true},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	byPath := filesByPath(t, files)
	model := byPath["microservices/services/system/internal/model/projects.go"]
	dao := byPath["microservices/services/system/internal/dao/system/projects.go"]
	service := byPath["microservices/services/system/internal/service/system/projects.go"]
	if !strings.Contains(model, "TenantID") || !strings.Contains(dao, `Where("tenant_id = ?"`) ||
		!strings.Contains(dao, "row.TenantID = tenant.FromContextOrDefault(ctx)") ||
		!strings.Contains(dao, `Where("(name LIKE ?)", like)`) {
		t.Fatalf("tenant boundary missing\nmodel:\n%s\ndao:\n%s", model, dao)
	}
	if strings.Contains(service, `json:"tenant_id"`) || strings.Contains(service, "input.TenantID") {
		t.Fatalf("tenant_id leaked into generated input:\n%s", service)
	}
}

func TestCodegenUsesConfiguredAntDesignComponent(t *testing.T) {
	request := assetPlanRequest()
	request.Fields[0].Component = ComponentTextArea
	files, err := NewCodegenServiceWithDB(newCodegenTestDB(t)).Generate(request)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := filesByPath(t, files)["microservices/web/src/pages/system/assets/index.tsx"]
	if !strings.Contains(page, "<Input.TextArea") {
		t.Fatalf("textarea component not rendered:\n%s", page)
	}
}

func TestCodegenTreeUsesInferredAntDesignComponent(t *testing.T) {
	files, err := NewCodegenServiceWithDB(newTreeSubTestDB(t)).Generate(treeRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := filesByPath(t, files)["microservices/web/src/pages/system/category/index.tsx"]
	if !strings.Contains(page, "<Input.TextArea") {
		t.Fatalf("tree textarea component not rendered:\n%s", page)
	}
}

func TestCodegenDateComponentSerializesFormValue(t *testing.T) {
	db := newCodegenTestDB(t)
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS dated_assets (
		id INTEGER PRIMARY KEY,
		published_on DATE NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	files, err := NewCodegenServiceWithDB(db).Generate(GenerateRequest{
		Table: "dated_assets", Module: "datedassets",
		Fields: []FieldConfig{{Name: "published_on", InList: true, InForm: true}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := filesByPath(t, files)["microservices/web/src/pages/system/datedassets/index.tsx"]
	for _, want := range []string{"DatePicker", "dayjs(row.published_on)", "format('YYYY-MM-DD')"} {
		if !strings.Contains(page, want) {
			t.Fatalf("date page missing %q:\n%s", want, page)
		}
	}
}

func TestCodegenSubInputExcludesManagedModelFields(t *testing.T) {
	files, err := NewCodegenServiceWithDB(newTreeSubTestDB(t)).Generate(subRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	service := filesByPath(t, files)["microservices/services/system/internal/service/system/orders.go"]
	if !strings.Contains(service, "type OrdersItemInput struct") || !strings.Contains(service, "func toOrdersItems(") {
		t.Fatalf("safe child input mapping missing:\n%s", service)
	}
	if strings.Contains(service, "Items []localmodel.DemoOrderItem") {
		t.Fatalf("child model leaked into request input:\n%s", service)
	}
}

func TestCodegenEscapesPermissionMigrationTitle(t *testing.T) {
	request := assetPlanRequest()
	request.Title = "O'Reilly 资产"
	files, err := NewCodegenServiceWithDB(newCodegenTestDB(t)).Generate(request)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	migration := filesByPath(t, files)["microservices/services/monitor/migrations/000000_codegen_assets.sql"]
	if !strings.Contains(migration, "O''Reilly 资产查看") || strings.Contains(migration, "'O'Reilly") {
		t.Fatalf("migration title is not SQL escaped:\n%s", migration)
	}
}

func sortedFilePaths(files []GeneratedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	slices.Sort(paths)
	return paths
}
