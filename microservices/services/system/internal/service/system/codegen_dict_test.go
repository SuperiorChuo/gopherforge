package system

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func seedCodegenDictTypes(t *testing.T, db *gorm.DB, codes ...string) {
	t.Helper()
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS dict_types (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE
	)`).Error; err != nil {
		t.Fatalf("create dict_types: %v", err)
	}
	for _, code := range codes {
		if err := db.Exec(`INSERT OR IGNORE INTO dict_types (code) VALUES (?)`, code).Error; err != nil {
			t.Fatalf("seed dict type %s: %v", code, err)
		}
	}
}

// TestCodegenDictMapping 测试枚举字典映射功能：字段配置 DictType 后，
// 生成的前端代码应包含字典加载、Select 表单组件、列表映射函数。
func TestCodegenDictMapping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 建一张测试表：id + status(int) + gender(int)
	if err := db.Exec(`CREATE TABLE test_users (
		id INTEGER PRIMARY KEY,
		name TEXT,
		status INTEGER,
		gender INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	seedCodegenDictTypes(t, db, "sys_common_status", "sys_user_sex")

	svc := NewCodegenServiceWithDB(db)
	req := GenerateRequest{
		Table:   "test_users",
		Module:  "testuser",
		Title:   "测试用户",
		TplType: TplTypeCRUD,
		Fields: []FieldConfig{
			{Name: "name", Label: "姓名", InList: true, InForm: true},
			{Name: "status", Label: "状态", InList: true, InForm: true, DictType: "sys_common_status"},
			{Name: "gender", Label: "性别", InList: true, InForm: true, DictType: "sys_user_sex"},
		},
	}

	files, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	byPath := make(map[string]string)
	for _, f := range files {
		byPath[f.Path] = f.Content
	}

	content, ok := byPath["microservices/web/src/pages/system/testuser/index.tsx"]
	if !ok {
		t.Fatal("web/src/pages/testuser/index.tsx not generated")
	}

	// 检查1：导入字典 API
	if !strings.Contains(content, "import { getDictItems, type DictItem } from '@/api/dict'") {
		t.Error("missing dict API import")
	}

	// 检查2：字典状态和加载逻辑
	if !strings.Contains(content, "const [dictData, setDictData] = useState<Record<string, DictItem[]>>({})") {
		t.Error("missing dictData state")
	}
	if !strings.Contains(content, "'sys_common_status'") {
		t.Error("missing sys_common_status in types array")
	}
	if !strings.Contains(content, "'sys_user_sex'") {
		t.Error("missing sys_user_sex in types array")
	}
	if !strings.Contains(content, "const getDictLabel = (type: string, value: any): string") {
		t.Error("missing getDictLabel function")
	}

	// 检查3：表单字段用 Select
	if !strings.Contains(content, "<Select options={dictData['sys_common_status']?.map(d => ({ label: d.label, value: d.value }))}") {
		t.Error("status field not rendered as Select with dict options")
	}
	if !strings.Contains(content, "<Select options={dictData['sys_user_sex']?.map(d => ({ label: d.label, value: d.value }))}") {
		t.Error("gender field not rendered as Select with dict options")
	}

	// 检查4：列表列用 getDictLabel 映射
	if !strings.Contains(content, "render: (v: any) => getDictLabel('sys_common_status', v)") {
		t.Error("status column not using getDictLabel")
	}
	if !strings.Contains(content, "render: (v: any) => getDictLabel('sys_user_sex', v)") {
		t.Error("gender column not using getDictLabel")
	}

	// 检查5：非字典字段（name）仍用 Input
	if !strings.Contains(content, `<Form.Item name="name" label="姓名"`) {
		t.Error("name field missing")
	}
	if strings.Contains(content, `dictData['name']`) {
		t.Error("name field should not use dict (it has no DictType)")
	}
}

// TestCodegenTreeDictMapping 树表模式也支持字典映射。
func TestCodegenTreeDictMapping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_categories (
		id INTEGER PRIMARY KEY,
		name TEXT,
		parent_id INTEGER,
		status INTEGER,
		sort INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	seedCodegenDictTypes(t, db, "sys_common_status")

	svc := NewCodegenServiceWithDB(db)
	req := GenerateRequest{
		Table:   "test_categories",
		Module:  "testcategory",
		Title:   "分类",
		TplType: TplTypeTree,
		Tree:    &TreeConfig{ParentField: "parent_id", NameField: "name", SortField: "sort"},
		Fields: []FieldConfig{
			{Name: "name", Label: "名称", InList: true, InForm: true},
			{Name: "status", Label: "状态", InList: true, InForm: true, DictType: "sys_common_status"},
			{Name: "sort", Label: "排序", InList: true, InForm: true},
		},
	}

	files, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	byPath := make(map[string]string)
	for _, f := range files {
		byPath[f.Path] = f.Content
	}

	content, ok := byPath["microservices/web/src/pages/system/testcategory/index.tsx"]
	if !ok {
		t.Fatal("web/src/pages/testcategory/index.tsx not generated")
	}

	if !strings.Contains(content, "import { getDictItems, type DictItem } from '@/api/dict'") {
		t.Error("tree mode: missing dict API import")
	}
	if !strings.Contains(content, "getDictLabel('sys_common_status', v)") {
		t.Error("tree mode: status column not using getDictLabel")
	}
	if !strings.Contains(content, "dictData['sys_common_status']?.map") {
		t.Error("tree mode: status field not using dict Select")
	}
}

// TestCodegenSubDictMapping 主子表模式也支持字典映射。
func TestCodegenSubDictMapping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_orders (
		id INTEGER PRIMARY KEY,
		order_no TEXT,
		status INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_order_items (
		id INTEGER PRIMARY KEY,
		order_id INTEGER REFERENCES test_orders(id),
		product_name TEXT,
		quantity INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	seedCodegenDictTypes(t, db, "order_status")

	svc := NewCodegenServiceWithDB(db)
	req := GenerateRequest{
		Table:   "test_orders",
		Module:  "testorder",
		Title:   "订单",
		TplType: TplTypeSub,
		Sub:     &SubConfig{Table: "test_order_items", FKField: "order_id"},
		Fields: []FieldConfig{
			{Name: "order_no", Label: "订单号", InList: true, InForm: true},
			{Name: "status", Label: "状态", InList: true, InForm: true, DictType: "order_status"},
		},
	}

	files, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	byPath := make(map[string]string)
	for _, f := range files {
		byPath[f.Path] = f.Content
	}

	content, ok := byPath["microservices/web/src/pages/system/testorder/index.tsx"]
	if !ok {
		t.Fatal("web/src/pages/testorder/index.tsx not generated")
	}

	if !strings.Contains(content, "import { getDictItems, type DictItem } from '@/api/dict'") {
		t.Error("sub mode: missing dict API import")
	}
	if !strings.Contains(content, "'order_status'") {
		t.Error("sub mode: missing order_status in types array")
	}
	if !strings.Contains(content, "getDictLabel('order_status', v)") {
		t.Error("sub mode: status column not using getDictLabel")
	}
}
