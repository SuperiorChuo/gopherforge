package system

var tplLayeredModel = mustTpl("layered-model", `package localmodel

import (
{{- if or .ModelHasTime .SubHasTime .SubHasAudit}}
	"time"
{{- end}}
{{- if or .HasDeleted .SubHasDeleted}}
	"gorm.io/gorm"
{{- end}}
)

// {{.Entity}} maps the {{.Table}} table.
type {{.Entity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
{{- if .HasTenant}}
	TenantID uint {{bq}}gorm:"column:tenant_id;index" json:"tenant_id"{{bq}}
{{- end}}
{{- if .IsTree}}
	{{.ParentCol.GoField}} uint64 {{bq}}gorm:"column:{{.ParentCol.Name}};index" json:"{{.ParentCol.Name}}"{{bq}}
{{- end}}
{{- range .ModelFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
{{- range .M2Ms}}
	{{exportedName .Name}}IDs []uint64 {{bq}}gorm:"-" json:"{{.Name}}_ids,omitempty"{{bq}}
{{- end}}
{{- if .IsTree}}
	Children []{{.Entity}} {{bq}}gorm:"-" json:"children,omitempty"{{bq}}
{{- end}}
{{- if .IsSub}}
	Items []{{.SubEntity}} {{bq}}gorm:"-" json:"items,omitempty"{{bq}}
{{- end}}
{{- if .HasCreated}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
{{- end}}
{{- if .HasUpdated}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
{{- end}}
{{- if .HasDeleted}}
	DeletedAt gorm.DeletedAt {{bq}}gorm:"index" json:"-"{{bq}}
{{- end}}
}

func ({{.Entity}}) TableName() string { return "{{.Table}}" }
{{- if .SubTable}}

// {{.SubEntity}} maps child rows from {{.SubTable}}.
type {{.SubEntity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
{{- if .SubHasTenant}}
	TenantID uint {{bq}}gorm:"column:tenant_id;index" json:"tenant_id"{{bq}}
{{- end}}
	{{.SubFKCol.GoField}} uint64 {{bq}}gorm:"column:{{.SubFKCol.Name}};index" json:"{{.SubFKCol.Name}}"{{bq}}
{{- range .SubFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
{{- if .SubHasAudit}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
{{- end}}
{{- if .SubHasDeleted}}
	DeletedAt gorm.DeletedAt {{bq}}gorm:"index" json:"-"{{bq}}
{{- end}}
}

func ({{.SubEntity}}) TableName() string { return "{{.SubTable}}" }
{{- end}}
`)
