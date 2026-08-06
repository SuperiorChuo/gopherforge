package system

var tplLayeredDAO = mustTpl("layered-dao", `package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-admin-kit/services/system/internal/model"
{{- if .NeedsTenant}}
	"github.com/go-admin-kit/services/shared/pkg/tenant"
{{- end}}
	"gorm.io/gorm"
)

type {{.ModuleType}}DAO struct { db *gorm.DB }

func New{{.ModuleType}}DAO(db *gorm.DB) *{{.ModuleType}}DAO { return &{{.ModuleType}}DAO{db: db} }

func (d *{{.ModuleType}}DAO) baseDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	query := db.WithContext(ctx).Model(&model.{{.Entity}}{})
{{- if .HasTenant}}
	query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
	return query
}

func (d *{{.ModuleType}}DAO) base(ctx context.Context) *gorm.DB { return d.baseDB(ctx, d.db) }

func (d *{{.ModuleType}}DAO) List(ctx context.Context, keyword string, page, pageSize int) ([]model.{{.Entity}}, int64, error) {
	if page <= 0 { page = 1 }
	if pageSize <= 0 || pageSize > 100 { pageSize = 20 }
	query := d.base(ctx)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
{{- if .SearchStr}}
		query = query.Where("({{range $index, $field := .SearchStr}}{{if $index}} OR {{end}}{{$field.Name}} LIKE ?{{end}})"{{range .SearchStr}}, like{{end}})
{{- else}}
		_ = like
{{- end}}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil { return nil, 0, err }
	var rows []model.{{.Entity}}
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil { return nil, 0, err }
{{- if .M2Ms}}
	if err := d.loadRelations(ctx, d.db, rows); err != nil { return nil, 0, err }
{{- end}}
	return rows, total, nil
}

func (d *{{.ModuleType}}DAO) Get(ctx context.Context, id uint64) (*model.{{.Entity}}, error) {
	var row model.{{.Entity}}
	if err := d.base(ctx).Where("id = ?", id).First(&row).Error; err != nil { return nil, err }
{{- if .M2Ms}}
	rows := []model.{{.Entity}}{row}
	if err := d.loadRelations(ctx, d.db, rows); err != nil { return nil, err }
	row = rows[0]
{{- end}}
{{- if .IsSub}}
	query := d.db.WithContext(ctx).Where("{{.SubFKCol.Name}} = ?", id)
{{- if .SubHasTenant}}
	query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
	if err := query.Find(&row.Items).Error; err != nil { return nil, err }
{{- end}}
	return &row, nil
}

func (d *{{.ModuleType}}DAO) Create(ctx context.Context, row *model.{{.Entity}}) error {
{{- if or .IsSub .M2Ms}}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
{{- if .HasTenant}}
		row.TenantID = tenant.FromContextOrDefault(ctx)
{{- end}}
		if err := tx.Create(row).Error; err != nil { return err }
{{- if .IsSub}}
		if err := d.replaceItems(ctx, tx, row); err != nil { return err }
{{- end}}
{{- if .M2Ms}}
		if err := d.replaceRelations(ctx, tx, row); err != nil { return err }
{{- end}}
		return nil
	})
{{- else}}
{{- if .HasTenant}}
	row.TenantID = tenant.FromContextOrDefault(ctx)
{{- end}}
	return d.db.WithContext(ctx).Create(row).Error
{{- end}}
}

func (d *{{.ModuleType}}DAO) Update(ctx context.Context, row *model.{{.Entity}}) error {
{{- if or .IsSub .M2Ms}}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := d.baseDB(ctx, tx).Where("id = ?", row.ID).Updates(row)
		if result.Error != nil { return result.Error }
		if result.RowsAffected == 0 { return gorm.ErrRecordNotFound }
{{- if .IsSub}}
		if err := d.replaceItems(ctx, tx, row); err != nil { return err }
{{- end}}
{{- if .M2Ms}}
		if err := d.replaceRelations(ctx, tx, row); err != nil { return err }
{{- end}}
		return nil
	})
{{- else}}
	result := d.base(ctx).Where("id = ?", row.ID).Updates(row)
	if result.Error != nil { return result.Error }
	if result.RowsAffected == 0 { return gorm.ErrRecordNotFound }
	return nil
{{- end}}
}

func (d *{{.ModuleType}}DAO) Delete(ctx context.Context, id uint64) error {
{{- if or .IsSub .M2Ms}}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
{{- if .IsSub}}
		items := tx.WithContext(ctx).Where("{{.SubFKCol.Name}} = ?", id)
{{- if .SubHasTenant}}
		items = items.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
		if err := items.Delete(&model.{{.SubEntity}}{}).Error; err != nil { return err }
{{- end}}
{{- range .M2Ms}}
		join := tx.WithContext(ctx).Table("{{.JoinTable}}").Where("{{.FKField}} = ?", id)
{{- if .JoinHasTenant}}
		join = join.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
		if err := join.Delete(nil).Error; err != nil { return err }
{{- end}}
		result := d.baseDB(ctx, tx).Where("id = ?", id).Delete(&model.{{.Entity}}{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected == 0 { return gorm.ErrRecordNotFound }
		return nil
	})
{{- else}}
	result := d.base(ctx).Where("id = ?", id).Delete(&model.{{.Entity}}{})
	if result.Error != nil { return result.Error }
	if result.RowsAffected == 0 { return gorm.ErrRecordNotFound }
	return nil
{{- end}}
}
{{- if .IsTree}}

func (d *{{.ModuleType}}DAO) Tree(ctx context.Context) ([]model.{{.Entity}}, error) {
	var rows []model.{{.Entity}}
	if err := d.base(ctx).Order("{{.TreeOrder}}").Find(&rows).Error; err != nil { return nil, err }
	children := make(map[uint64][]model.{{.Entity}})
	for _, row := range rows { children[row.{{.ParentCol.GoField}}] = append(children[row.{{.ParentCol.GoField}}], row) }
	var build func(uint64) []model.{{.Entity}}
	build = func(parent uint64) []model.{{.Entity}} {
		nodes := children[parent]
		for index := range nodes { nodes[index].Children = build(nodes[index].ID) }
		return nodes
	}
	return build(0), nil
}

func (d *{{.ModuleType}}DAO) HasChildren(ctx context.Context, id uint64) (bool, error) {
	var count int64
	err := d.base(ctx).Where("{{.ParentCol.Name}} = ?", id).Count(&count).Error
	return count > 0, err
}
{{- end}}
{{- if .IsSub}}

func (d *{{.ModuleType}}DAO) replaceItems(ctx context.Context, tx *gorm.DB, row *model.{{.Entity}}) error {
	query := tx.WithContext(ctx).Where("{{.SubFKCol.Name}} = ?", row.ID)
{{- if .SubHasTenant}}
	query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
	if err := query.Delete(&model.{{.SubEntity}}{}).Error; err != nil { return err }
	for index := range row.Items {
		row.Items[index].ID = 0
		row.Items[index].{{.SubFKCol.GoField}} = row.ID
{{- if .SubHasTenant}}
		row.Items[index].TenantID = tenant.FromContextOrDefault(ctx)
{{- end}}
	}
	if len(row.Items) == 0 { return nil }
	return tx.WithContext(ctx).Create(&row.Items).Error
}
{{- end}}
{{- if .M2Ms}}

func (d *{{.ModuleType}}DAO) loadRelations(ctx context.Context, db *gorm.DB, rows []model.{{.Entity}}) error {
	if len(rows) == 0 { return nil }
	ids := make([]uint64, len(rows)); positions := make(map[uint64]int, len(rows))
	for index := range rows { ids[index] = rows[index].ID; positions[rows[index].ID] = index }
{{- range .M2Ms}}
	var {{.Name}}Links []struct {
		SourceID uint64 {{bq}}gorm:"column:{{.FKField}}"{{bq}}
		TargetID uint64 {{bq}}gorm:"column:{{.TargetFK}}"{{bq}}
	}
	query := db.WithContext(ctx).Table("{{.JoinTable}}").Where("{{.FKField}} IN ?", ids)
{{- if .JoinHasTenant}}
	query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
	if err := query.Find(&{{.Name}}Links).Error; err != nil { return err }
	for _, link := range {{.Name}}Links { index := positions[link.SourceID]; rows[index].{{exportedName .Name}}IDs = append(rows[index].{{exportedName .Name}}IDs, link.TargetID) }
{{- end}}
	return nil
}

func (d *{{.ModuleType}}DAO) replaceRelations(ctx context.Context, tx *gorm.DB, row *model.{{.Entity}}) error {
{{- range .M2Ms}}
	if len(row.{{exportedName .Name}}IDs) > 0 {
		query := tx.WithContext(ctx).Table("{{.TargetTable}}").Where("id IN ?", row.{{exportedName .Name}}IDs)
{{- if .TargetHasTenant}}
		query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
		var count int64
		if err := query.Count(&count).Error; err != nil { return err }
		if count != int64(len(row.{{exportedName .Name}}IDs)) { return fmt.Errorf("invalid {{.Name}} relation ids") }
	}
	join := tx.WithContext(ctx).Table("{{.JoinTable}}").Where("{{.FKField}} = ?", row.ID)
{{- if .JoinHasTenant}}
	join = join.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
	if err := join.Delete(nil).Error; err != nil { return err }
	if len(row.{{exportedName .Name}}IDs) > 0 {
		values := make([]map[string]any, 0, len(row.{{exportedName .Name}}IDs))
		for _, targetID := range row.{{exportedName .Name}}IDs {
			value := map[string]any{"{{.FKField}}": row.ID, "{{.TargetFK}}": targetID}
{{- if .JoinHasTenant}}
			value["tenant_id"] = tenant.FromContextOrDefault(ctx)
{{- end}}
			values = append(values, value)
		}
		if err := tx.WithContext(ctx).Table("{{.JoinTable}}").Create(&values).Error; err != nil { return err }
	}
{{- end}}
	return nil
}
{{- end}}

type {{.ModuleType}}RelationOption struct {
	Value uint64 {{bq}}json:"value"{{bq}}
	Label string {{bq}}json:"label"{{bq}}
}

func (d *{{.ModuleType}}DAO) RelationOptions(ctx context.Context, name string) ([]{{.ModuleType}}RelationOption, error) {
	var options []{{.ModuleType}}RelationOption
{{- if not .M2Ms}}
	_ = options
{{- end}}
	switch name {
{{- range .M2Ms}}
	case "{{.Name}}":
		query := d.db.WithContext(ctx).Table("{{.TargetTable}}")
{{- if .TargetHasTenant}}
		query = query.Where("tenant_id = ?", tenant.FromContextOrDefault(ctx))
{{- end}}
		err := query.Select("id AS value, {{.DisplayField}} AS label").Order("{{.DisplayField}} ASC").Scan(&options).Error
		return options, err
{{- end}}
	default:
		return nil, fmt.Errorf("unknown relation %q", name)
	}
}
`)
