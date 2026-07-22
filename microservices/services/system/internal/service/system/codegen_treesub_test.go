package system

// 树表 / 主子表生成模式的快照类测试：Go 产物必须能通过 go/parser 语法
// 解析，TSX 产物做定界符残留与括号配平的语法层断言；另含单表模式回归
// 用例（空 tpl_type 与显式 crud 产物必须逐字节一致，且无任何新模式痕迹）。

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTreeSubTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 独立的共享内存库名，避免污染 codegen_test.go 的表列表
	db, err := gorm.Open(sqlite.Open("file:codegen_treesub?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS demo_categories (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			parent_id INTEGER NOT NULL DEFAULT 0,
			sort INTEGER,
			remark TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS demo_orders (
			id INTEGER PRIMARY KEY,
			order_no TEXT NOT NULL,
			remark TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS demo_order_items (
			id INTEGER PRIMARY KEY,
			order_id INTEGER NOT NULL,
			sku TEXT,
			qty INTEGER,
			price REAL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		// 无可生成列的子表（只有主键+外键+审计列），用于校验报错
		`CREATE TABLE IF NOT EXISTS demo_bare_items (
			id INTEGER PRIMARY KEY,
			order_id INTEGER NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// mustParseGo 断言生成的 Go 产物是语法合法的 Go 源码。
func mustParseGo(t *testing.T, path, src string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), path, src, parser.AllErrors); err != nil {
		t.Fatalf("生成的 %s 不是合法 Go 代码: %v\n%s", path, err, src)
	}
}

// checkTSX 对 ts/tsx 产物做语法层的基本断言：模板定界符不残留、括号配平。
func checkTSX(t *testing.T, path, src string) {
	t.Helper()
	for _, delim := range []string{"[[", "]]"} {
		if strings.Contains(src, delim) {
			t.Fatalf("%s 残留模板定界符 %q:\n%s", path, delim, src)
		}
	}
	for _, pair := range [][2]rune{{'{', '}'}, {'(', ')'}, {'[', ']'}} {
		open := strings.Count(src, string(pair[0]))
		closed := strings.Count(src, string(pair[1]))
		if open != closed {
			t.Fatalf("%s 括号不配平 %c=%d %c=%d", path, pair[0], open, pair[1], closed)
		}
	}
}

func filesByPath(t *testing.T, files []GeneratedFile) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Content
	}
	return byPath
}

// verifyArtifacts 断言产物路径齐全，并对 Go/TSX 做语法层校验。
func verifyArtifacts(t *testing.T, byPath map[string]string, module string) {
	t.Helper()
	for _, p := range []string{
		"server/" + module + "/model.go", "server/" + module + "/store.go",
		"server/" + module + "/handlers.go", "server/" + module + "/routes.go",
		"web/src/api/" + module + ".ts", "web/src/pages/" + module + "/index.tsx",
		"menu-" + module + ".sql",
	} {
		if byPath[p] == "" {
			t.Fatalf("missing artifact %s (have %v)", p, keys(byPath))
		}
		switch {
		case strings.HasSuffix(p, ".go"):
			mustParseGo(t, p, byPath[p])
		case strings.HasSuffix(p, ".ts"), strings.HasSuffix(p, ".tsx"):
			checkTSX(t, p, byPath[p])
		}
	}
}

func treeRequest() GenerateRequest {
	return GenerateRequest{
		Table:   "demo_categories",
		Module:  "category",
		Title:   "分类管理",
		TplType: TplTypeTree,
		Tree:    &TreeConfig{ParentField: "parent_id", NameField: "name", SortField: "sort"},
		Fields: []FieldConfig{
			{Name: "name", Label: "名称", InList: true, InSearch: true, InForm: true, Required: true},
			{Name: "parent_id", Label: "父级", InList: true, InForm: true}, // 父级列应被模板特殊处理，不重复出现
			{Name: "sort", Label: "排序", InList: true, InForm: true},
			{Name: "remark", Label: "备注", InForm: true},
		},
	}
}

func TestCodegenGenerateTree(t *testing.T) {
	svc := NewCodegenServiceWithDB(newTreeSubTestDB(t))
	files, err := svc.Generate(treeRequest())
	if err != nil {
		t.Fatalf("Generate(tree): %v", err)
	}
	byPath := filesByPath(t, files)
	verifyArtifacts(t, byPath, "category")

	model := byPath["server/category/model.go"]
	for _, want := range []string{
		"type Category struct",
		"ParentID uint64",
		"Children []Category",
		`gorm:"-" json:"children,omitempty"`,
		`return "demo_categories"`,
	} {
		if !strings.Contains(model, want) {
			t.Fatalf("tree model.go missing %q:\n%s", want, model)
		}
	}
	// 父级列只能出现一次（模板显式渲染 + 字段循环去重）
	if strings.Count(model, "ParentID uint64") != 1 {
		t.Fatalf("tree model.go duplicated parent field:\n%s", model)
	}

	store := byPath["server/category/store.go"]
	for _, want := range []string{
		"func buildTree(", "func (s *Store) Tree()",
		"ErrHasChildren", "parent_id ASC, sort ASC, id ASC",
		"name LIKE ?", // 平铺列表的关键字搜索仍在
	} {
		if !strings.Contains(store, want) {
			t.Fatalf("tree store.go missing %q:\n%s", want, store)
		}
	}

	handlers := byPath["server/category/handlers.go"]
	for _, want := range []string{"func (s *Server) Tree(", "不能选择自己作为父级", "父节点不存在"} {
		if !strings.Contains(handlers, want) {
			t.Fatalf("tree handlers.go missing %q", want)
		}
	}

	routes := byPath["server/category/routes.go"]
	if !strings.Contains(routes, `r.GET("/api/v1/category/tree", s.Tree)`) {
		t.Fatalf("tree routes.go missing tree route:\n%s", routes)
	}

	api := byPath["web/src/api/category.ts"]
	for _, want := range []string{"children?: Category[]", "getCategoryTree", "parent_id: number"} {
		if !strings.Contains(api, want) {
			t.Fatalf("tree api.ts missing %q:\n%s", want, api)
		}
	}

	page := byPath["web/src/pages/category/index.tsx"]
	for _, want := range []string{
		"TreeSelect", "treeData={treeSelectData}", "defaultExpandAllRows",
		"toTreeSelectData", "分类管理", "新建下级",
		"n.value !== editing?.id", // 编辑时剪掉自己防成环
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("tree page missing %q", want)
		}
	}
}

func TestCodegenTreeValidation(t *testing.T) {
	svc := NewCodegenServiceWithDB(newTreeSubTestDB(t))
	base := treeRequest()

	for name, mutate := range map[string]func(*GenerateRequest){
		"缺 tree 配置":  func(r *GenerateRequest) { r.Tree = nil },
		"父级字段不存在":    func(r *GenerateRequest) { r.Tree = &TreeConfig{ParentField: "ghost", NameField: "name"} },
		"父级字段非整数":    func(r *GenerateRequest) { r.Tree = &TreeConfig{ParentField: "remark", NameField: "name"} },
		"显示字段未选":     func(r *GenerateRequest) { r.Tree = &TreeConfig{ParentField: "parent_id", NameField: "ghost"} },
		"显示字段非文本":    func(r *GenerateRequest) { r.Tree = &TreeConfig{ParentField: "parent_id", NameField: "sort"} },
		"排序字段不存在":    func(r *GenerateRequest) { r.Tree.SortField = "ghost" },
	} {
		req := base
		req.Tree = &TreeConfig{ParentField: base.Tree.ParentField, NameField: base.Tree.NameField, SortField: base.Tree.SortField}
		mutate(&req)
		if _, err := svc.Generate(req); err == nil {
			t.Fatalf("tree case %q should fail", name)
		}
	}
}

func subRequest() GenerateRequest {
	return GenerateRequest{
		Table:   "demo_orders",
		Module:  "orders",
		Title:   "订单管理",
		TplType: TplTypeSub,
		Sub:     &SubConfig{Table: "demo_order_items", FKField: "order_id"},
		Fields: []FieldConfig{
			{Name: "order_no", Label: "订单号", InList: true, InSearch: true, InForm: true, Required: true},
			{Name: "remark", Label: "备注", InList: true, InForm: true},
		},
	}
}

func TestCodegenGenerateSub(t *testing.T) {
	svc := NewCodegenServiceWithDB(newTreeSubTestDB(t))
	files, err := svc.Generate(subRequest())
	if err != nil {
		t.Fatalf("Generate(sub): %v", err)
	}
	byPath := filesByPath(t, files)
	verifyArtifacts(t, byPath, "orders")

	model := byPath["server/orders/model.go"]
	for _, want := range []string{
		"type Order struct",
		"type DemoOrderItem struct",
		"Items []DemoOrderItem",
		"OrderID uint64",
		"Sku string", "Qty int64", "Price float64",
		`return "demo_order_items"`,
	} {
		if !strings.Contains(model, want) {
			t.Fatalf("sub model.go missing %q:\n%s", want, model)
		}
	}

	store := byPath["server/orders/store.go"]
	for _, want := range []string{
		"s.db.Transaction(", "func replaceItems(",
		`Where("order_id = ?", m.ID).Delete(&DemoOrderItem{})`, // 先删后插全量替换
		"m.Items[i].OrderID = m.ID",
	} {
		if !strings.Contains(store, want) {
			t.Fatalf("sub store.go missing %q:\n%s", want, store)
		}
	}
	// 删除也必须连子表同事务
	if !strings.Contains(store, `Where("order_id = ?", id).Delete(&DemoOrderItem{})`) {
		t.Fatalf("sub store.go Delete should remove items:\n%s", store)
	}

	handlers := byPath["server/orders/handlers.go"]
	for _, want := range []string{"type itemReq struct", "Items []itemReq", "func toItems(", "func (s *Server) Detail("} {
		if !strings.Contains(handlers, want) {
			t.Fatalf("sub handlers.go missing %q", want)
		}
	}

	routes := byPath["server/orders/routes.go"]
	if !strings.Contains(routes, `r.GET("/api/v1/orders/:id", s.Detail)`) {
		t.Fatalf("sub routes.go missing detail route:\n%s", routes)
	}

	api := byPath["web/src/api/orders.ts"]
	for _, want := range []string{
		"export type DemoOrderItem = {", "items?: DemoOrderItem[]",
		"export type OrderUpsert", "export function getOrder(",
	} {
		if !strings.Contains(api, want) {
			t.Fatalf("sub api.ts missing %q:\n%s", want, api)
		}
	}

	page := byPath["web/src/pages/orders/index.tsx"]
	for _, want := range []string{
		"itemColumns", "patchItem", "添加一行", "子表明细",
		"setItems(detail.items ?? [])", "订单管理",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("sub page missing %q", want)
		}
	}
}

func TestCodegenSubValidation(t *testing.T) {
	svc := NewCodegenServiceWithDB(newTreeSubTestDB(t))

	for name, mutate := range map[string]func(*GenerateRequest){
		"缺 sub 配置":  func(r *GenerateRequest) { r.Sub = nil },
		"子表不存在":     func(r *GenerateRequest) { r.Sub = &SubConfig{Table: "no_such", FKField: "order_id"} },
		"子表同主表":     func(r *GenerateRequest) { r.Sub = &SubConfig{Table: "demo_orders", FKField: "order_id"} },
		"外键不存在":     func(r *GenerateRequest) { r.Sub = &SubConfig{Table: "demo_order_items", FKField: "ghost"} },
		"外键非整数":     func(r *GenerateRequest) { r.Sub = &SubConfig{Table: "demo_order_items", FKField: "sku"} },
		"子表无可生成列":   func(r *GenerateRequest) { r.Sub = &SubConfig{Table: "demo_bare_items", FKField: "order_id"} },
	} {
		req := subRequest()
		mutate(&req)
		if _, err := svc.Generate(req); err == nil {
			t.Fatalf("sub case %q should fail", name)
		}
	}
}

// TestCodegenSingleTableRegression 证明单表模式行为未变：
// 空 tpl_type 与显式 crud 产物逐字节一致、产物集合不变、无新模式痕迹，
// 且 Go 产物语法合法（配合 codegen_test.go 里未改动的既有断言共同兜底）。
func TestCodegenSingleTableRegression(t *testing.T) {
	svc := NewCodegenServiceWithDB(newTreeSubTestDB(t))
	req := GenerateRequest{
		Table:  "demo_orders",
		Module: "orders",
		Title:  "订单",
		Fields: []FieldConfig{
			{Name: "order_no", Label: "订单号", InList: true, InSearch: true, InForm: true, Required: true},
			{Name: "remark", Label: "备注", InList: true, InForm: true},
		},
	}
	implicit, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate(默认): %v", err)
	}
	req.TplType = TplTypeCRUD
	explicit, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate(crud): %v", err)
	}
	if len(implicit) != len(explicit) || len(implicit) != 7 {
		t.Fatalf("artifact count changed: implicit=%d explicit=%d", len(implicit), len(explicit))
	}
	for i := range implicit {
		if implicit[i].Path != explicit[i].Path || implicit[i].Content != explicit[i].Content {
			t.Fatalf("空 tpl_type 与 crud 产物不一致: %s", implicit[i].Path)
		}
	}
	byPath := filesByPath(t, implicit)
	verifyArtifacts(t, byPath, "orders")
	// 单表产物不得混入树表/主子表模式的任何痕迹
	for path, content := range byPath {
		for _, marker := range []string{"Children", "children", "buildTree", "TreeSelect", "Items", "items", "replaceItems", "Transaction"} {
			if strings.Contains(content, marker) {
				t.Fatalf("单表产物 %s 混入新模式痕迹 %q", path, marker)
			}
		}
	}
	// 未知模式必须拒绝
	req.TplType = "ghost"
	if _, err := svc.Generate(req); err == nil {
		t.Fatal("unknown tpl_type should be rejected")
	}
}
