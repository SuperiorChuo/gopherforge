package system

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newCodegenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:codegen?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS demo_assets (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		amount_cents INTEGER,
		active BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

func TestCodegenIntrospection(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenTestDB(t))

	tables, err := svc.ListTables()
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "demo_assets" {
			found = true
		}
	}
	if !found {
		t.Fatalf("demo_assets missing from %v", tables)
	}

	cols, err := svc.TableColumns("demo_assets")
	if err != nil {
		t.Fatalf("TableColumns: %v", err)
	}
	byName := map[string]ColumnInfo{}
	for _, c := range cols {
		byName[c.Name] = c
	}
	if byName["amount_cents"].GoType != "int64" || byName["amount_cents"].TSType != "number" {
		t.Fatalf("amount_cents mapping = %+v", byName["amount_cents"])
	}
	if byName["name"].GoField != "Name" || byName["amount_cents"].GoField != "AmountCents" {
		t.Fatalf("field naming wrong: %+v", byName)
	}
	if _, err := svc.TableColumns("no_such_table"); err == nil {
		t.Fatal("unknown table should error")
	}
}

func TestCodegenGenerate(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenTestDB(t))
	files, err := svc.Generate(GenerateRequest{
		Table:  "demo_assets",
		Module: "assets",
		Title:  "资产管理",
		Fields: []FieldConfig{
			{Name: "name", Label: "名称", InList: true, InSearch: true, InForm: true, Required: true},
			{Name: "amount_cents", Label: "金额(分)", InList: true, InForm: true},
			{Name: "active", Label: "启用", InList: true, InForm: true},
			{Name: "id"},         // primary key must be dropped
			{Name: "created_at"}, // audit column must be dropped
			{Name: "ghost"},      // unknown column must be dropped
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Content
	}
	for _, p := range []string{
		"server/assets/model.go", "server/assets/store.go",
		"server/assets/handlers.go", "server/assets/routes.go",
		"web/src/api/assets.ts", "web/src/pages/assets/index.tsx",
		"menu-assets.sql",
	} {
		if byPath[p] == "" {
			t.Fatalf("missing artifact %s (have %v)", p, keys(byPath))
		}
	}

	model := byPath["server/assets/model.go"]
	for _, want := range []string{
		"type Asset struct", "AmountCents int64", "Active bool",
		"`gorm:\"column:name\" json:\"name\"`", `return "demo_assets"`,
	} {
		if !strings.Contains(model, want) {
			t.Fatalf("model.go missing %q:\n%s", want, model)
		}
	}
	if strings.Contains(model, "Ghost") || strings.Contains(model, "CreatedAt time.Time `gorm") {
		t.Fatalf("model.go leaked dropped fields:\n%s", model)
	}

	store := byPath["server/assets/store.go"]
	if !strings.Contains(store, "name LIKE ?") {
		t.Fatalf("store.go missing keyword search:\n%s", store)
	}

	page := byPath["web/src/pages/assets/index.tsx"]
	for _, want := range []string{"资产管理", "title: '名称'", "rules={[{ required: true }]}", "<Switch />"} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q", want)
		}
	}

	api := byPath["web/src/api/assets.ts"]
	if !strings.Contains(api, "export type Asset = {") || !strings.Contains(api, "amount_cents: number") {
		t.Fatalf("api.ts wrong:\n%s", api)
	}
}

func TestCodegenModuleValidation(t *testing.T) {
	svc := NewCodegenServiceWithDB(newCodegenTestDB(t))
	for _, bad := range []string{"", "a-b", "1abc", "a b", "x"} {
		if _, err := svc.Generate(GenerateRequest{Table: "demo_assets", Module: bad,
			Fields: []FieldConfig{{Name: "name", InForm: true}}}); err == nil {
			t.Fatalf("module %q should be rejected", bad)
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
