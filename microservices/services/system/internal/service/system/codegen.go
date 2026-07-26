package system

// Code generator: introspects PostgreSQL tables via the gorm migrator and
// renders a CRUD starter kit (Go experimental-line service files, a React
// list page, an axios api module and a menu seed SQL) from text templates.
// Generated output is a download artifact, not compiled into this repo.

import (
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"

	"gorm.io/gorm"
)

// CodegenService generates CRUD scaffolding from database tables.
type CodegenService struct {
	DB *gorm.DB
}

func NewCodegenServiceWithDB(db *gorm.DB) CodegenService {
	return CodegenService{DB: db}
}

// internal tables never offered for generation
var codegenExcluded = map[string]bool{
	"goose_db_version": true,
}

// ListTables returns table names ordered alphabetically.
func (s CodegenService) ListTables() ([]TableInfo, error) {
	schemas, err := NewSchemaInspector(s.DB).InspectTables()
	if err != nil {
		return nil, err
	}
	out := make([]TableInfo, 0, len(schemas))
	for _, schema := range schemas {
		if schema.PrimaryKey == "" {
			continue
		}
		out = append(out, TableInfo{
			Name:          schema.Name,
			Comment:       schema.Comment,
			PrimaryKey:    schema.PrimaryKey,
			ColumnCount:   len(schema.Columns),
			RelationCount: len(schema.Relations),
		})
	}
	return out, nil
}

// TableColumns introspects one table.
func (s CodegenService) TableColumns(table string) ([]ColumnInfo, error) {
	schema, err := NewSchemaInspector(s.DB).InspectTable(table)
	if err != nil {
		return nil, err
	}
	return schema.Columns, nil
}

func goTypeOf(dbType string) string {
	switch {
	case strings.HasPrefix(dbType, "int"), strings.HasPrefix(dbType, "serial"),
		dbType == "bigint", dbType == "smallint", dbType == "integer":
		return "int64"
	case strings.HasPrefix(dbType, "numeric"), strings.HasPrefix(dbType, "decimal"),
		strings.HasPrefix(dbType, "float"), strings.HasPrefix(dbType, "double"), dbType == "real":
		return "float64"
	case dbType == "bool", dbType == "boolean":
		return "bool"
	case strings.HasPrefix(dbType, "timestamp"), dbType == "date", strings.HasPrefix(dbType, "datetime"):
		return "time.Time"
	default:
		return "string"
	}
}

func tsTypeOf(dbType string) string {
	switch goTypeOf(dbType) {
	case "int64", "float64":
		return "number"
	case "bool":
		return "boolean"
	default:
		return "string"
	}
}

// exportedName converts snake_case to ExportedCamelCase, keeping common
// initialisms readable (id -> ID, url -> URL, ip -> IP).
func exportedName(snake string) string {
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		switch p {
		case "id":
			parts[i] = "ID"
		case "url":
			parts[i] = "URL"
		case "ip":
			parts[i] = "IP"
		case "api":
			parts[i] = "API"
		default:
			if p != "" {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
	}
	return strings.Join(parts, "")
}

// camelName is exportedName with a lowered first rune (for TS identifiers).
func camelName(snake string) string {
	e := exportedName(snake)
	if e == "" {
		return e
	}
	return strings.ToLower(e[:1]) + e[1:]
}

// tplField is the enriched field passed to templates.
type tplField struct {
	FieldConfig
	Column ColumnInfo
}

type tplM2M struct {
	M2MConfig
	TargetHasTenant bool
	JoinHasTenant   bool
}

type tplData struct {
	Table          string
	Module         string // url segment, e.g. asset
	Title          string // human title
	Entity         string // Go type name, e.g. Asset
	EntityLower    string // e.g. asset
	ModuleType     string
	Fields         []tplField // configured, non-audit fields
	ModelFields    []tplField
	ListFields     []tplField
	SearchStr      []tplField // string search fields
	FormFields     []tplField
	HasTime        bool
	ModelHasTime   bool
	HasDateForm    bool
	HasNumberInput bool
	HasSwitchInput bool
	HasSelectInput bool
	HasTagList     bool
	DictTypes      []string // 去重后的字典类型列表（前端需加载的字典）
	M2Ms           []tplM2M // 多对多关联配置（用于生成关联字段和加载逻辑）
	HasTenant      bool
	HasCreated     bool
	HasUpdated     bool
	HasDeleted     bool
	NeedsTenant    bool
	IsTree         bool
	IsSub          bool

	// 树表模式专用（约定与部门树一致：后端组树 + 平铺分页列表并存）
	ParentCol      ColumnInfo // 父级列，模型中恒为 uint64 与 ID 对齐
	NameField      tplField   // 显示字段（树节点标题），必须在字段配置中
	TreeListFields []tplField // 列表列（剔除显示字段，显示字段固定放首列）
	TreeOrder      string     // 树查询排序表达式，如 "parent_id ASC, sort ASC, id ASC"

	// 主子表模式专用
	SubTable       string
	SubEntity      string     // 子表 Go 类型名，如 OrderItem
	SubEntityLower string     // 如 orderItem
	SubFKCol       ColumnInfo // 子表外键列，模型中恒为 uint64
	SubFields      []tplField // 子表可生成字段（自动全选，不含主键/外键/审计列）
	SubHasAudit    bool       // 子表是否同时具备 created_at/updated_at
	SubHasTime     bool       // 子表字段是否含时间列
	SubHasTenant   bool
	SubHasDeleted  bool
}

// Generate renders all artifacts. Column metadata is re-introspected so the
// client cannot inject arbitrary type text into templates.
func (s CodegenService) Generate(req GenerateRequest) ([]GeneratedFile, error) {
	validated, err := s.ValidateRequest(req)
	if err != nil {
		return nil, err
	}
	return s.generateValidated(validated)
}

func (s CodegenService) generateValidated(validated ValidatedRequest) ([]GeneratedFile, error) {
	req := validated.Request
	tplType := req.TplType
	cols := validated.Schema.Columns
	byName := map[string]ColumnInfo{}
	data := tplData{
		Table:       req.Table,
		Module:      req.Module,
		Title:       strings.TrimSpace(req.Title),
		Entity:      exportedName(singular(req.Module)),
		EntityLower: camelName(singular(req.Module)),
		ModuleType:  exportedName(req.Module),
		IsTree:      tplType == TplTypeTree,
		IsSub:       tplType == TplTypeSub,
	}
	for _, c := range cols {
		byName[c.Name] = c
		if c.GoType == "time.Time" {
			data.ModelHasTime = true
		}
		if tplType == TplTypeTree && req.Tree != nil && c.Name == req.Tree.ParentField {
			continue
		}
		switch c.Name {
		case "tenant_id":
			data.HasTenant = true
			data.NeedsTenant = true
		case "created_at":
			data.HasCreated = true
		case "updated_at":
			data.HasUpdated = true
		case "deleted_at":
			data.HasDeleted = true
		case validated.Schema.PrimaryKey:
		default:
			data.ModelFields = append(data.ModelFields, tplField{
				FieldConfig: FieldConfig{Name: c.Name, Label: c.Label},
				Column:      c,
			})
		}
	}
	if data.Title == "" {
		data.Title = data.Entity
	}
	// 树表的父级列不进普通字段循环：模型/表单里由模板特殊处理（TreeSelect）
	skip := map[string]bool{}
	if tplType == TplTypeTree {
		if req.Tree == nil || req.Tree.ParentField == "" || req.Tree.NameField == "" {
			return nil, fmt.Errorf("tree mode requires parent_field and name_field")
		}
		pcol, ok := byName[req.Tree.ParentField]
		if !ok {
			return nil, fmt.Errorf("parent field %q not found in table %s", req.Tree.ParentField, req.Table)
		}
		if pcol.GoType != "int64" || pcol.PrimaryKey {
			return nil, fmt.Errorf("parent field %q must be a non-primary integer column", req.Tree.ParentField)
		}
		data.ParentCol = pcol
		skip[pcol.Name] = true
	}
	for _, f := range req.Fields {
		col := byName[f.Name]
		if col.PrimaryKey || isServerManagedColumn(f.Name) {
			continue // id / created_at / updated_at handled by templates
		}
		if skip[f.Name] {
			continue
		}
		if f.Label == "" {
			f.Label = f.Name
		}
		tf := tplField{FieldConfig: f, Column: col}
		data.Fields = append(data.Fields, tf)
		if f.InList {
			data.ListFields = append(data.ListFields, tf)
		}
		if f.InSearch && col.GoType == "string" {
			data.SearchStr = append(data.SearchStr, tf)
		}
		if f.InForm {
			data.FormFields = append(data.FormFields, tf)
			if f.Component == ComponentDate || f.Component == ComponentDateTime {
				data.HasDateForm = true
			}
		}
		if col.GoType == "time.Time" {
			data.HasTime = true
		}
	}
	if len(data.Fields) == 0 {
		return nil, fmt.Errorf("no generatable fields selected")
	}

	// 收集去重后的字典类型（前端需加载）
	dictSet := map[string]bool{}
	for _, f := range data.Fields {
		if dt := strings.TrimSpace(f.DictType); dt != "" {
			dictSet[dt] = true
		}
	}
	for dt := range dictSet {
		data.DictTypes = append(data.DictTypes, dt)
	}
	sort.Strings(data.DictTypes) // 稳定生成顺序

	// 填充多对多关联配置（单表/树表可用，主子表不支持）
	if tplType != TplTypeSub && len(req.M2Ms) > 0 {
		for _, m2m := range req.M2Ms {
			if m2m.Name == "" || m2m.JoinTable == "" || m2m.FKField == "" ||
				m2m.TargetTable == "" || m2m.TargetFK == "" || m2m.DisplayField == "" {
				return nil, fmt.Errorf("m2m config incomplete: all fields required")
			}
			if m2m.Label == "" {
				m2m.Label = m2m.Name // 默认用 name 作为前端标签
			}
			target, _ := findTableSchema(validated.Schemas, m2m.TargetTable)
			join, _ := findTableSchema(validated.Schemas, m2m.JoinTable)
			data.M2Ms = append(data.M2Ms, tplM2M{
				M2MConfig:       m2m,
				TargetHasTenant: target.HasColumn("tenant_id"),
				JoinHasTenant:   join.HasColumn("tenant_id"),
			})
			data.NeedsTenant = data.NeedsTenant || target.HasColumn("tenant_id") || join.HasColumn("tenant_id")
		}
	}

	switch tplType {
	case TplTypeTree:
		if err := s.fillTreeData(&data, req.Tree, byName); err != nil {
			return nil, err
		}
	case TplTypeSub:
		if err := s.fillSubData(&data, req.Sub, req.Table, validated.SubSchema); err != nil {
			return nil, err
		}
	}
	for _, field := range data.FormFields {
		data.HasNumberInput = data.HasNumberInput || field.Component == ComponentNumber
		data.HasSwitchInput = data.HasSwitchInput || field.Component == ComponentSwitch
		data.HasSelectInput = data.HasSelectInput || strings.TrimSpace(field.DictType) != ""
	}
	for _, field := range data.SubFields {
		data.HasNumberInput = data.HasNumberInput || field.Column.TSType == "number"
		data.HasSwitchInput = data.HasSwitchInput || field.Column.TSType == "boolean"
	}
	if len(data.M2Ms) > 0 {
		data.HasSelectInput = true
		data.HasTagList = true
	}

	var out []GeneratedFile
	for _, t := range templateSet(tplType, req.Module) {
		var b strings.Builder
		if err := t.tpl.Execute(&b, data); err != nil {
			return nil, fmt.Errorf("render %s: %w", t.path, err)
		}
		content := b.String()
		if strings.HasSuffix(t.path, ".go") {
			formatted, err := format.Source([]byte(content))
			if err != nil {
				return nil, fmt.Errorf("format %s: %w", t.path, err)
			}
			content = string(formatted)
		}
		out = append(out, GeneratedFile{Path: t.path, Content: content})
	}
	return out, nil
}

type tplEntry struct {
	path string
	tpl  *template.Template
}

// templateSet selects layered system-service and admin-web artifacts.
func templateSet(tplType, module string) []tplEntry {
	page := tplPage
	webAPI := tplAPI
	switch tplType {
	case TplTypeTree:
		page = tplTreePage
		webAPI = tplTreeAPI
	case TplTypeSub:
		page = tplSubPage
		webAPI = tplSubAPI
	}
	return []tplEntry{
		{fmt.Sprintf("microservices/services/system/internal/model/%s.go", module), tplLayeredModel},
		{fmt.Sprintf("microservices/services/system/internal/dao/system/%s.go", module), tplLayeredDAO},
		{fmt.Sprintf("microservices/services/system/internal/service/system/%s.go", module), tplLayeredService},
		{fmt.Sprintf("microservices/services/system/internal/api/system/%s.go", module), tplLayeredAPI},
		{fmt.Sprintf("microservices/web/src/api/%s.ts", module), webAPI},
		{fmt.Sprintf("microservices/web/src/pages/system/%s/index.tsx", module), page},
		{fmt.Sprintf("microservices/services/monitor/migrations/000000_codegen_%s.sql", module), tplPermissionMigration},
	}
}

// fillTreeData 校验并补齐树表专用模板数据。约定与部门管理一致：
// 后端 buildTree 组树、树接口返回整棵树、平铺列表接口并存。
func (s CodegenService) fillTreeData(data *tplData, cfg *TreeConfig, byName map[string]ColumnInfo) error {
	var nameField *tplField
	for i := range data.Fields {
		if data.Fields[i].Name == cfg.NameField {
			nameField = &data.Fields[i]
		}
	}
	if nameField == nil {
		return fmt.Errorf("name field %q must be a selected non-audit column", cfg.NameField)
	}
	if nameField.Column.GoType != "string" {
		return fmt.Errorf("name field %q must be a text column", cfg.NameField)
	}
	data.NameField = *nameField
	// 显示字段固定在首列，列表列里去重
	for _, f := range data.ListFields {
		if f.Name != cfg.NameField {
			data.TreeListFields = append(data.TreeListFields, f)
		}
	}
	order := fmt.Sprintf("%s ASC", cfg.ParentField)
	if cfg.SortField != "" {
		scol, ok := byName[cfg.SortField]
		if !ok {
			return fmt.Errorf("sort field %q not found", cfg.SortField)
		}
		if scol.PrimaryKey || scol.Name == cfg.ParentField {
			return fmt.Errorf("sort field %q must be a regular column", cfg.SortField)
		}
		order += fmt.Sprintf(", %s ASC", cfg.SortField)
	}
	data.TreeOrder = order + ", id ASC"
	return nil
}

// fillSubData uses the validated child-table snapshot and field configuration.
// Saving child rows still follows full replacement semantics.
func (s CodegenService) fillSubData(data *tplData, cfg *SubConfig, mainTable string, schema *TableSchema) error {
	if cfg == nil || cfg.Table == "" || cfg.FKField == "" {
		return fmt.Errorf("sub mode requires sub table and fk_field")
	}
	if cfg.Table == mainTable {
		return fmt.Errorf("sub table must differ from main table")
	}
	if schema == nil || schema.Name != cfg.Table {
		return fmt.Errorf("sub table metadata is missing for %s", cfg.Table)
	}
	subCols := schema.Columns
	var (
		fk         *ColumnInfo
		hasCreated bool
		hasUpdated bool
	)
	for i, c := range subCols {
		switch c.Name {
		case "tenant_id":
			data.SubHasTenant = true
			data.NeedsTenant = true
		case "created_at":
			hasCreated = true
		case "updated_at":
			hasUpdated = true
		case "deleted_at":
			data.SubHasDeleted = true
		}
		if c.Name == cfg.FKField {
			fk = &subCols[i]
		}
	}
	if fk == nil {
		return fmt.Errorf("fk field %q not found in sub table %s", cfg.FKField, cfg.Table)
	}
	if fk.GoType != "int64" || fk.PrimaryKey {
		return fmt.Errorf("fk field %q must be a non-primary integer column", cfg.FKField)
	}
	data.SubTable = cfg.Table
	data.SubEntity = exportedName(singular(cfg.Table))
	data.SubEntityLower = camelName(singular(cfg.Table))
	if data.SubEntity == data.Entity {
		// 子表实体与主表实体撞名时加后缀，避免生成重复类型
		data.SubEntity += "Item"
		data.SubEntityLower += "Item"
	}
	data.SubFKCol = *fk
	data.SubHasAudit = hasCreated && hasUpdated
	byName := make(map[string]ColumnInfo, len(subCols))
	for _, column := range subCols {
		byName[column.Name] = column
	}
	for _, field := range cfg.Fields {
		c := byName[field.Name]
		data.SubFields = append(data.SubFields, tplField{
			FieldConfig: field.FieldConfig,
			Column:      c,
		})
		if c.GoType == "time.Time" {
			data.SubHasTime = true
		}
	}
	if len(data.SubFields) == 0 {
		return fmt.Errorf("sub table %s has no generatable columns", cfg.Table)
	}
	return nil
}

func isAuditColumn(name string) bool {
	switch name {
	case "id", "created_at", "updated_at", "deleted_at":
		return true
	}
	return false
}

func isServerManagedColumn(name string) bool {
	return name == "tenant_id" || isAuditColumn(name)
}

// singular chops a trailing "s" for a nicer entity name (assets -> asset).
func singular(s string) string {
	if strings.HasSuffix(s, "es") && len(s) > 3 {
		return strings.TrimSuffix(s, "es")
	}
	if strings.HasSuffix(s, "s") && len(s) > 2 {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

var moduleRe = mustRe(`^[a-z][a-z0-9]{1,31}$`)
var relationNameRe = mustRe(`^[a-z][a-z0-9_]{1,31}$`)
