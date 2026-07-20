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

// GenerateRequest is the full generation config.
type GenerateRequest struct {
	Table  string        `json:"table"`
	Module string        `json:"module"` // e.g. "asset" -> route /api/v1/asset/...
	Title  string        `json:"title"`  // e.g. Chinese page title
	Fields []FieldConfig `json:"fields"`
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
	for _, f := range req.Fields {
		col, ok := byName[f.Name]
		if !ok {
			continue // silently drop unknown fields
		}
		if col.PrimaryKey || isAuditColumn(f.Name) {
			continue // id / created_at / updated_at handled by templates
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

	var out []GeneratedFile
	for _, t := range []struct {
		path string
		tpl  *template.Template
	}{
		{fmt.Sprintf("server/%s/model.go", req.Module), tplModel},
		{fmt.Sprintf("server/%s/store.go", req.Module), tplStore},
		{fmt.Sprintf("server/%s/handlers.go", req.Module), tplHandlers},
		{fmt.Sprintf("server/%s/routes.go", req.Module), tplRoutes},
		{fmt.Sprintf("web/src/api/%s.ts", req.Module), tplAPI},
		{fmt.Sprintf("web/src/pages/%s/index.tsx", req.Module), tplPage},
		{fmt.Sprintf("menu-%s.sql", req.Module), tplMenu},
	} {
		var b strings.Builder
		if err := t.tpl.Execute(&b, data); err != nil {
			return nil, fmt.Errorf("render %s: %w", t.path, err)
		}
		out = append(out, GeneratedFile{Path: t.path, Content: b.String()})
	}
	return out, nil
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
