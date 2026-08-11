package system

var tplLayeredService = mustTpl("layered-service", `package system

import (
	"context"
{{- if .IsTree}}
	"errors"
{{- end}}
{{- if .HasTime}}
	"time"
{{- end}}

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

type {{.ModuleType}}Service struct { dao *systemdao.{{.ModuleType}}DAO }

func New{{.ModuleType}}Service(db *gorm.DB) *{{.ModuleType}}Service {
	return &{{.ModuleType}}Service{dao: systemdao.New{{.ModuleType}}DAO(db)}
}
{{- if .IsSub}}

type {{.ModuleType}}ItemInput struct {
{{- range .SubFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
}

func to{{.ModuleType}}Items(inputs []{{.ModuleType}}ItemInput) []localmodel.{{.SubEntity}} {
	items := make([]localmodel.{{.SubEntity}}, len(inputs))
	for index, input := range inputs {
		items[index] = localmodel.{{.SubEntity}}{
{{- range .SubFields}}
			{{.Column.GoField}}: input.{{.Column.GoField}},
{{- end}}
		}
	}
	return items
}
{{- end}}

type {{.ModuleType}}Input struct {
{{- if .IsTree}}
	{{.ParentCol.GoField}} uint64 {{bq}}json:"{{.ParentCol.Name}}"{{bq}}
{{- end}}
{{- range .FormFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
{{- range .M2Ms}}
	{{exportedName .Name}}IDs []uint64 {{bq}}json:"{{.Name}}_ids"{{bq}}
{{- end}}
{{- if .IsSub}}
	Items []{{.ModuleType}}ItemInput {{bq}}json:"items"{{bq}}
{{- end}}
}

func (s *{{.ModuleType}}Service) List(ctx context.Context, keyword string, page, pageSize int) ([]localmodel.{{.Entity}}, int64, error) {
	return s.dao.List(ctx, keyword, page, pageSize)
}

func (s *{{.ModuleType}}Service) Get(ctx context.Context, id uint64) (*localmodel.{{.Entity}}, error) {
	return s.dao.Get(ctx, id)
}
{{- if .IsTree}}

func (s *{{.ModuleType}}Service) Tree(ctx context.Context) ([]localmodel.{{.Entity}}, error) { return s.dao.Tree(ctx) }
{{- end}}

func (s *{{.ModuleType}}Service) Create(ctx context.Context, input {{.ModuleType}}Input) (*localmodel.{{.Entity}}, error) {
	row := &localmodel.{{.Entity}}{
{{- if .IsTree}}
		{{.ParentCol.GoField}}: input.{{.ParentCol.GoField}},
{{- end}}
{{- range .FormFields}}
		{{.Column.GoField}}: input.{{.Column.GoField}},
{{- end}}
{{- range .M2Ms}}
		{{exportedName .Name}}IDs: input.{{exportedName .Name}}IDs,
{{- end}}
{{- if .IsSub}}
		Items: to{{.ModuleType}}Items(input.Items),
{{- end}}
	}
{{- if .IsTree}}
	if row.{{.ParentCol.GoField}} != 0 { if _, err := s.dao.Get(ctx, row.{{.ParentCol.GoField}}); err != nil { return nil, err } }
{{- end}}
	if err := s.dao.Create(ctx, row); err != nil { return nil, err }
	return row, nil
}

func (s *{{.ModuleType}}Service) Update(ctx context.Context, id uint64, input {{.ModuleType}}Input) (*localmodel.{{.Entity}}, error) {
	row, err := s.dao.Get(ctx, id)
	if err != nil { return nil, err }
{{- if .IsTree}}
	if input.{{.ParentCol.GoField}} == id { return nil, errors.New("不能选择自己作为父级") }
	if input.{{.ParentCol.GoField}} != 0 { if _, err := s.dao.Get(ctx, input.{{.ParentCol.GoField}}); err != nil { return nil, errors.New("父节点不存在") } }
	row.{{.ParentCol.GoField}} = input.{{.ParentCol.GoField}}
{{- end}}
{{- range .FormFields}}
	row.{{.Column.GoField}} = input.{{.Column.GoField}}
{{- end}}
{{- range .M2Ms}}
	row.{{exportedName .Name}}IDs = input.{{exportedName .Name}}IDs
{{- end}}
{{- if .IsSub}}
	row.Items = to{{.ModuleType}}Items(input.Items)
{{- end}}
	if err := s.dao.Update(ctx, row); err != nil { return nil, err }
	return row, nil
}

func (s *{{.ModuleType}}Service) Delete(ctx context.Context, id uint64) error {
{{- if .IsTree}}
	hasChildren, err := s.dao.HasChildren(ctx, id)
	if err != nil { return err }
	if hasChildren { return errors.New("存在子节点，不能删除") }
{{- end}}
	return s.dao.Delete(ctx, id)
}

func (s *{{.ModuleType}}Service) RelationOptions(ctx context.Context, name string) ([]systemdao.{{.ModuleType}}RelationOption, error) {
	return s.dao.RelationOptions(ctx, name)
}
`)
