package system

// Code generator: introspects PostgreSQL tables via the gorm migrator and
// renders a CRUD starter kit (Go experimental-line service files, a React
// list page, an axios api module and a menu seed SQL) from text templates.
// Generated output is a download artifact, not compiled into this repo.

import (
	"fmt"
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

// TableInfo is one candidate table.
type TableInfo struct {
	Name string `json:"name"`
}

// ColumnInfo describes one column with mapped Go / TS types.
type ColumnInfo struct {
	Name       string `json:"name"`
	DBType     string `json:"db_type"`
	GoType     string `json:"go_type"`
	TSType     string `json:"ts_type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	GoField    string `json:"go_field"`
	Label      string `json:"label"`
}

// FieldConfig is the per-field generation choice from the UI.
type FieldConfig struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	InList   bool   `json:"in_list"`
	InSearch bool   `json:"in_search"`
	InForm   bool   `json:"in_form"`
	Required bool   `json:"required"`
}

// 生成模式（借鉴 ruoyi-vue-pro 的模板类型）。
const (
	TplTypeCRUD = "crud" // 单表（默认，行为与历史版本完全一致）
	TplTypeTree = "tree" // 树表：父级字段自关联，列表返回整棵树
	TplTypeSub  = "sub"  // 主子表：主表 CRUD + 子表行同事务全量替换
)

// TreeConfig 树表模式配置。
type TreeConfig struct {
	ParentField string `json:"parent_field"` // 父级字段（如 parent_id），须为本表非主键整数列
	NameField   string `json:"name_field"`   // 显示字段（如 name），用作树节点标题
	SortField   string `json:"sort_field"`   // 可选排序字段（如 sort），空则退化为按 id 排序
}

// SubConfig 主子表模式配置。
type SubConfig struct {
	Table   string `json:"table"`    // 子表表名
	FKField string `json:"fk_field"` // 子表中指向主表 id 的外键列
}

// GenerateRequest is the full generation config.
type GenerateRequest struct {
	Table   string        `json:"table"`
	Module  string        `json:"module"`   // e.g. "asset" -> route /api/v1/asset/...
	Title   string        `json:"title"`    // e.g. Chinese page title
	TplType string        `json:"tpl_type"` // 生成模式：""/crud=单表，tree=树表，sub=主子表
	Tree    *TreeConfig   `json:"tree,omitempty"`
	Sub     *SubConfig    `json:"sub,omitempty"`
	Fields  []FieldConfig `json:"fields"`
}

// GeneratedFile is one rendered artifact.
type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// internal tables never offered for generation
var codegenExcluded = map[string]bool{
	"goose_db_version": true,
}

// ListTables returns table names ordered alphabetically.
func (s CodegenService) ListTables() ([]TableInfo, error) {
	names, err := s.DB.Migrator().GetTables()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]TableInfo, 0, len(names))
	for _, n := range names {
		if codegenExcluded[n] {
			continue
		}
		out = append(out, TableInfo{Name: n})
	}
	return out, nil
}

// TableColumns introspects one table.
func (s CodegenService) TableColumns(table string) ([]ColumnInfo, error) {
	if !s.DB.Migrator().HasTable(table) {
		return nil, fmt.Errorf("table %q not found", table)
	}
	cols, err := s.DB.Migrator().ColumnTypes(table)
	if err != nil {
		return nil, err
	}
	out := make([]ColumnInfo, 0, len(cols))
	for _, c := range cols {
		dbType := strings.ToLower(c.DatabaseTypeName())
		nullable, _ := c.Nullable()
		pk, _ := c.PrimaryKey()
		out = append(out, ColumnInfo{
			Name:       c.Name(),
			DBType:     dbType,
			GoType:     goTypeOf(dbType),
			TSType:     tsTypeOf(dbType),
			Nullable:   nullable,
			PrimaryKey: pk,
			GoField:    exportedName(c.Name()),
			Label:      c.Name(),
		})
	}
	return out, nil
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

type tplData struct {
	Table       string
	Module      string     // url segment, e.g. asset
	Title       string     // human title
	Entity      string     // Go type name, e.g. Asset
	EntityLower string     // e.g. asset
	Fields      []tplField // configured, non-audit fields
	ListFields  []tplField
	SearchStr   []tplField // string search fields
	FormFields  []tplField
	HasTime     bool

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
}

// Generate renders all artifacts. Column metadata is re-introspected so the
// client cannot inject arbitrary type text into templates.
func (s CodegenService) Generate(req GenerateRequest) ([]GeneratedFile, error) {
	req.Module = strings.ToLower(strings.TrimSpace(req.Module))
	if req.Table == "" || req.Module == "" {
		return nil, fmt.Errorf("table and module are required")
	}
	if !moduleRe.MatchString(req.Module) {
		return nil, fmt.Errorf("module must be lowercase letters/digits, starting with a letter")
	}
	tplType := req.TplType
	if tplType == "" {
		tplType = TplTypeCRUD
	}
	if tplType != TplTypeCRUD && tplType != TplTypeTree && tplType != TplTypeSub {
		return nil, fmt.Errorf("unknown tpl_type %q", req.TplType)
	}
	cols, err := s.TableColumns(req.Table)
	if err != nil {
		return nil, err
	}
	byName := map[string]ColumnInfo{}
	for _, c := range cols {
		byName[c.Name] = c
	}
	data := tplData{
		Table:       req.Table,
		Module:      req.Module,
		Title:       strings.TrimSpace(req.Title),
		Entity:      exportedName(singular(req.Module)),
		EntityLower: camelName(singular(req.Module)),
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
		col, ok := byName[f.Name]
		if !ok {
			continue // silently drop unknown fields
		}
		if col.PrimaryKey || isAuditColumn(f.Name) {
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
		}
		if col.GoType == "time.Time" {
			data.HasTime = true
		}
	}
	if len(data.Fields) == 0 {
		return nil, fmt.Errorf("no generatable fields selected")
	}

	switch tplType {
	case TplTypeTree:
		if err := s.fillTreeData(&data, req.Tree, byName); err != nil {
			return nil, err
		}
	case TplTypeSub:
		if err := s.fillSubData(&data, req.Sub, req.Table); err != nil {
			return nil, err
		}
	}

	var out []GeneratedFile
	for _, t := range templateSet(tplType, req.Module) {
		var b strings.Builder
		if err := t.tpl.Execute(&b, data); err != nil {
			return nil, fmt.Errorf("render %s: %w", t.path, err)
		}
		out = append(out, GeneratedFile{Path: t.path, Content: b.String()})
	}
	return out, nil
}

type tplEntry struct {
	path string
	tpl  *template.Template
}

// templateSet 按模式选模板；单表沿用历史模板，产物路径三种模式一致。
func templateSet(tplType, module string) []tplEntry {
	model, store, handlers, page := tplModel, tplStore, tplHandlers, tplPage
	api, routes := tplAPI, tplRoutes
	switch tplType {
	case TplTypeTree:
		model, store, handlers, page = tplTreeModel, tplTreeStore, tplTreeHandlers, tplTreePage
		api, routes = tplTreeAPI, tplTreeRoutes
	case TplTypeSub:
		model, store, handlers, page = tplSubModel, tplSubStore, tplSubHandlers, tplSubPage
		api, routes = tplSubAPI, tplSubRoutes
	}
	return []tplEntry{
		{fmt.Sprintf("server/%s/model.go", module), model},
		{fmt.Sprintf("server/%s/store.go", module), store},
		{fmt.Sprintf("server/%s/handlers.go", module), handlers},
		{fmt.Sprintf("server/%s/routes.go", module), routes},
		{fmt.Sprintf("web/src/api/%s.ts", module), api},
		{fmt.Sprintf("web/src/pages/%s/index.tsx", module), page},
		{fmt.Sprintf("menu-%s.sql", module), tplMenu},
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

// fillSubData 校验并补齐主子表专用模板数据。子表字段不走 UI 配置，
// 自动纳入全部非主键/非外键/非审计列（ruoyi 语义：保存时全量替换子表行）。
func (s CodegenService) fillSubData(data *tplData, cfg *SubConfig, mainTable string) error {
	if cfg == nil || cfg.Table == "" || cfg.FKField == "" {
		return fmt.Errorf("sub mode requires sub table and fk_field")
	}
	if cfg.Table == mainTable {
		return fmt.Errorf("sub table must differ from main table")
	}
	subCols, err := s.TableColumns(cfg.Table)
	if err != nil {
		return fmt.Errorf("sub table: %w", err)
	}
	var (
		fk         *ColumnInfo
		hasCreated bool
		hasUpdated bool
	)
	for i, c := range subCols {
		switch c.Name {
		case "created_at":
			hasCreated = true
		case "updated_at":
			hasUpdated = true
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
	for _, c := range subCols {
		if c.PrimaryKey || isAuditColumn(c.Name) || c.Name == cfg.FKField {
			continue
		}
		data.SubFields = append(data.SubFields, tplField{
			FieldConfig: FieldConfig{Name: c.Name, Label: c.Name, InList: true, InForm: true},
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
