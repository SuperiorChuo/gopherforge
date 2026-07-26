package system

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestCodegenM2MMapping 测试多对多关联功能：配置 M2Ms 后，
// 生成的后端代码包含关联字段、JOIN 查询、事务更新；
// 前端代码包含目标表加载、多选表单、Tag 显示。
func TestCodegenM2MMapping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 建三张表：users（主表）、user_roles（中间表）、roles（目标表）
	if err := db.Exec(`CREATE TABLE test_users (
		id INTEGER PRIMARY KEY,
		name TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_user_roles (
		user_id INTEGER REFERENCES test_users(id),
		role_id INTEGER REFERENCES test_roles(id)
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_roles (
		id INTEGER PRIMARY KEY,
		name TEXT
	)`).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewCodegenServiceWithDB(db)
	req := GenerateRequest{
		Table:   "test_users",
		Module:  "testuser",
		Title:   "测试用户",
		TplType: TplTypeCRUD,
		Fields: []FieldConfig{
			{Name: "name", Label: "姓名", InList: true, InForm: true},
		},
		M2Ms: []M2MConfig{
			{
				Name:         "roles",
				JoinTable:    "test_user_roles",
				FKField:      "user_id",
				TargetTable:  "test_roles",
				TargetFK:     "role_id",
				DisplayField: "name",
				Label:        "角色",
			},
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

	// 检查1：后端 Model 加 M2M 字段
	model := byPath["microservices/services/system/internal/model/testuser.go"]
	if model == "" {
		t.Fatal("model.go not generated")
	}
	if !strings.Contains(model, "RolesIDs") || !strings.Contains(model, "[]uint64") {
		t.Error("Model missing RolesIDs field")
	}
	if !strings.Contains(model, `gorm:"-" json:"roles_ids,omitempty"`) {
		t.Error("Model RolesIDs missing correct tags")
	}

	// 检查2：后端 Store List 加载关联
	store := byPath["microservices/services/system/internal/dao/system/testuser.go"]
	if store == "" {
		t.Fatal("store.go not generated")
	}
	if !strings.Contains(store, "func (d *TestuserDAO) loadRelations") {
		t.Error("Store List missing M2M loading logic")
	}
	if !strings.Contains(store, "test_user_roles") {
		t.Error("Store List not querying join table")
	}
	if !strings.Contains(store, "rows[index].RolesIDs = append") {
		t.Error("Store List not populating M2M IDs")
	}

	// 检查3：后端 Store Get 加载单个关联
	if !strings.Contains(store, `Table("test_user_roles").Where("user_id IN ?", ids)`) {
		t.Error("Store Get not loading M2M for single entity")
	}

	// 检查4：后端 Store Create 事务插入关联
	if !strings.Contains(store, "d.db.WithContext(ctx).Transaction") {
		t.Error("Store Create not using transaction for M2M")
	}
	if !strings.Contains(store, `Table("test_user_roles").Create(&values)`) {
		t.Error("Store Create not inserting M2M relations")
	}

	// 检查5：后端 Store Update 先删后插
	if !strings.Contains(store, `Table("test_user_roles").Where("user_id = ?", row.ID)`) || !strings.Contains(store, "join.Delete(nil)") {
		t.Error("Store Update not deleting old M2M relations")
	}

	// 检查6：后端 Handler upsertReq 加 M2M IDs
	handlers := byPath["microservices/services/system/internal/service/system/testuser.go"]
	if handlers == "" {
		t.Fatal("handlers.go not generated")
	}
	if !strings.Contains(handlers, "RolesIDs") || !strings.Contains(handlers, `json:"roles_ids"`) {
		t.Error("Handler upsertReq missing RolesIDs field")
	}
	if !strings.Contains(handlers, "RolesIDs: input.RolesIDs") {
		t.Error("Handler Create not passing M2M IDs to model")
	}
	if !strings.Contains(handlers, "row.RolesIDs = input.RolesIDs") {
		t.Error("Handler Update not updating M2M IDs")
	}

	// 检查7：前端 API 类型加 M2M IDs
	api := byPath["microservices/web/src/api/testuser.ts"]
	if api == "" {
		t.Fatal("api ts not generated")
	}
	if !strings.Contains(api, "roles_ids?: number[]") {
		t.Error("Frontend API type missing roles_ids field")
	}

	// 检查8：前端 Page 加载目标表选项
	page := byPath["microservices/web/src/pages/system/testuser/index.tsx"]
	if page == "" {
		t.Fatal("page tsx not generated")
	}
	if !strings.Contains(page, "const [ rolesOptions, setRolesOptions] = useState") {
		t.Error("Frontend Page missing roles options state")
	}
	if !strings.Contains(page, "const loadM2MTargets = async") {
		t.Error("Frontend Page missing M2M target loading logic")
	}

	// 检查9：前端列表列用 Tag 显示
	if !strings.Contains(page, `title: '角色', dataIndex: 'roles_ids'`) {
		t.Error("Frontend Page missing roles column")
	}
	if !strings.Contains(page, "<Tag key={id}>{opt.label}</Tag>") {
		t.Error("Frontend Page not rendering M2M as Tags")
	}

	// 检查10：前端表单用多选 Select
	if !strings.Contains(page, `<Form.Item name="roles_ids" label="角色">`) {
		t.Error("Frontend Page missing roles form field")
	}
	if !strings.Contains(page, `<Select mode="multiple" options={rolesOptions}`) {
		t.Error("Frontend Page not using multiple Select for M2M")
	}
}

// TestCodegenTreeM2M 树表模式也支持多对多关联。
func TestCodegenTreeM2M(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_categories (
		id INTEGER PRIMARY KEY,
		name TEXT,
		parent_id INTEGER,
		sort INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_category_tags (
		category_id INTEGER REFERENCES test_categories(id),
		tag_id INTEGER REFERENCES test_tags(id)
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE test_tags (
		id INTEGER PRIMARY KEY,
		name TEXT
	)`).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewCodegenServiceWithDB(db)
	req := GenerateRequest{
		Table:   "test_categories",
		Module:  "testcategory",
		Title:   "分类",
		TplType: TplTypeTree,
		Tree:    &TreeConfig{ParentField: "parent_id", NameField: "name", SortField: "sort"},
		Fields: []FieldConfig{
			{Name: "name", Label: "名称", InList: true, InForm: true},
			{Name: "sort", Label: "排序", InList: true, InForm: true},
		},
		M2Ms: []M2MConfig{
			{
				Name:         "tags",
				JoinTable:    "test_category_tags",
				FKField:      "category_id",
				TargetTable:  "test_tags",
				TargetFK:     "tag_id",
				DisplayField: "name",
				Label:        "标签",
			},
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

	model := byPath["microservices/services/system/internal/model/testcategory.go"]
	if !strings.Contains(model, "TagsIDs") || !strings.Contains(model, "[]uint64") {
		t.Error("Tree mode: Model missing TagsIDs field")
	}

	page := byPath["microservices/web/src/pages/system/testcategory/index.tsx"]
	if !strings.Contains(page, "tagsOptions") {
		t.Error("Tree mode: Page missing tags options")
	}
	if !strings.Contains(page, `<Select mode="multiple" options={tagsOptions}`) {
		t.Error("Tree mode: Page not using multiple Select for M2M")
	}
}
