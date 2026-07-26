package system

// 主子表模式模板。语义借鉴 ruoyi-vue-pro：主表 CRUD + 详情带子表行、
// 创建/更新在同一事务里全量替换子表行（先删后插）、删除连同子表行。
// 子表字段由数据库自动推导（去掉主键/外键/审计列），前端在弹窗里用
// 行内可编辑表格增删子表行。单表模板见 codegen_templates.go。

var tplSubModel = mustTpl("sub_model", `package {{.Module}}

import "time"

// {{.Entity}} maps table {{.Table}}（主表）.
type {{.Entity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
{{- range .Fields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
{{- range .M2Ms}}
	{{exportedName .Name}}IDs []uint64 {{bq}}gorm:"-" json:"{{.Name}}_ids,omitempty"{{bq}}
{{- end}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
	// Items 子表行，不落库；详情接口带出，保存时全量替换（ruoyi 语义）
	Items []{{.SubEntity}} {{bq}}gorm:"-" json:"items"{{bq}}
}

func ({{.Entity}}) TableName() string { return "{{.Table}}" }

// {{.SubEntity}} maps table {{.SubTable}}（子表，{{.SubFKCol.Name}} 指向主表 id）.
type {{.SubEntity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
	{{.SubFKCol.GoField}} uint64 {{bq}}gorm:"column:{{.SubFKCol.Name}}" json:"{{.SubFKCol.Name}}"{{bq}}
{{- range .SubFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
{{- if .SubHasAudit}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
{{- end}}
}

func ({{.SubEntity}}) TableName() string { return "{{.SubTable}}" }
`)

var tplSubStore = mustTpl("sub_store", `package {{.Module}}

import (
	"strings"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

type ListFilter struct {
	Keyword  string
	Page     int
	PageSize int
}

func (f *ListFilter) clamp() (offset, limit int) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}
	return (f.Page - 1) * f.PageSize, f.PageSize
}

// List 仅查主表（子表行走 Get 详情）。
func (s *Store) List(f ListFilter) ([]{{.Entity}}, int64, error) {
	q := s.db.Model(&{{.Entity}}{})
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		_ = like
{{- if .SearchStr}}
		q = q.Where("{{range $i, $f := .SearchStr}}{{if $i}} OR {{end}}{{$f.Name}} LIKE ?{{end}}"{{range .SearchStr}}, like{{end}})
{{- end}}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.clamp()
	var list []{{.Entity}}
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
{{- if .M2Ms}}
	// 批量加载多对多关联
	if len(list) > 0 {
		ids := make([]uint64, len(list))
		for i := range list {
			ids[i] = list[i].ID
		}
{{- range .M2Ms}}
		type {{exportedName .Name}}Rel struct {
			{{exportedName $.Module}}ID uint64 {{bq}}gorm:"column:{{.FKField}}"{{bq}}
			{{exportedName .Name}}ID uint64 {{bq}}gorm:"column:{{.TargetFK}}"{{bq}}
		}
		var {{.Name}}Rels []{{exportedName .Name}}Rel
		if err := s.db.Table("{{.JoinTable}}").Where("{{.FKField}} IN ?", ids).Find(&{{.Name}}Rels).Error; err == nil {
			{{.Name}}Map := make(map[uint64][]uint64)
			for _, r := range {{.Name}}Rels {
				{{.Name}}Map[r.{{exportedName $.Module}}ID] = append({{.Name}}Map[r.{{exportedName $.Module}}ID], r.{{exportedName .Name}}ID)
			}
			for i := range list {
				list[i].{{exportedName .Name}}IDs = {{.Name}}Map[list[i].ID]
			}
		}
{{- end}}
	}
{{- end}}
	return list, total, err
}

// Get 返回主表记录并带出全部子表行。
func (s *Store) Get(id uint64) (*{{.Entity}}, error) {
	var m {{.Entity}}
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Where("{{.SubFKCol.Name}} = ?", id).Order("id ASC").Find(&m.Items).Error; err != nil {
		return nil, err
	}
{{- if .M2Ms}}
	// 加载多对多关联
{{- range .M2Ms}}
	var {{.Name}}IDs []uint64
	if err := s.db.Table("{{.JoinTable}}").Where("{{.FKField}} = ?", id).Pluck("{{.TargetFK}}", &{{.Name}}IDs).Error; err == nil {
		m.{{exportedName .Name}}IDs = {{.Name}}IDs
	}
{{- end}}
{{- end}}
	return &m, nil
}

// Create 主子表同事务：先建主表拿 id，再写子表行。
func (s *Store) Create(m *{{.Entity}}) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		if err := replaceItems(tx, m); err != nil {
			return err
		}
{{- if .M2Ms}}
		// 保存多对多关联
{{- range .M2Ms}}
		if len(m.{{exportedName .Name}}IDs) > 0 {
			type {{exportedName .Name}}Rel struct {
				{{exportedName $.Module}}ID uint64 {{bq}}gorm:"column:{{.FKField}}"{{bq}}
				{{exportedName .Name}}ID uint64 {{bq}}gorm:"column:{{.TargetFK}}"{{bq}}
			}
			rels := make([]{{exportedName .Name}}Rel, len(m.{{exportedName .Name}}IDs))
			for i, tid := range m.{{exportedName .Name}}IDs {
				rels[i] = {{exportedName .Name}}Rel{ {{exportedName $.Module}}ID: m.ID, {{exportedName .Name}}ID: tid }
			}
			if err := tx.Table("{{.JoinTable}}").Create(&rels).Error; err != nil {
				return err
			}
		}
{{- end}}
{{- end}}
		return nil
	})
}

// Update 主子表同事务：更新主表后全量替换子表行（ruoyi 语义：先删后插）。
func (s *Store) Update(m *{{.Entity}}) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(m).Error; err != nil {
			return err
		}
		if err := replaceItems(tx, m); err != nil {
			return err
		}
{{- if .M2Ms}}
		// 更新多对多关联（先删旧的再插入新的）
{{- range .M2Ms}}
		if err := tx.Exec("DELETE FROM {{.JoinTable}} WHERE {{.FKField}} = ?", m.ID).Error; err != nil {
			return err
		}
		if len(m.{{exportedName .Name}}IDs) > 0 {
			type {{exportedName .Name}}Rel struct {
				{{exportedName $.Module}}ID uint64 {{bq}}gorm:"column:{{.FKField}}"{{bq}}
				{{exportedName .Name}}ID uint64 {{bq}}gorm:"column:{{.TargetFK}}"{{bq}}
			}
			rels := make([]{{exportedName .Name}}Rel, len(m.{{exportedName .Name}}IDs))
			for i, tid := range m.{{exportedName .Name}}IDs {
				rels[i] = {{exportedName .Name}}Rel{ {{exportedName $.Module}}ID: m.ID, {{exportedName .Name}}ID: tid }
			}
			if err := tx.Table("{{.JoinTable}}").Create(&rels).Error; err != nil {
				return err
			}
		}
{{- end}}
{{- end}}
		return nil
	})
}

// replaceItems 删除主表现有子表行后按当前 Items 重建，外键统一回填。
func replaceItems(tx *gorm.DB, m *{{.Entity}}) error {
	if err := tx.Where("{{.SubFKCol.Name}} = ?", m.ID).Delete(&{{.SubEntity}}{}).Error; err != nil {
		return err
	}
	if len(m.Items) == 0 {
		return nil
	}
	for i := range m.Items {
		m.Items[i].ID = 0
		m.Items[i].{{.SubFKCol.GoField}} = m.ID
	}
	return tx.Create(&m.Items).Error
}

// Delete 主子表同事务删除。
func (s *Store) Delete(id uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("{{.SubFKCol.Name}} = ?", id).Delete(&{{.SubEntity}}{}).Error; err != nil {
			return err
		}
		return tx.Delete(&{{.Entity}}{}, id).Error
	})
}
`)

var tplSubHandlers = mustTpl("sub_handlers", `package {{.Module}}

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Server struct {
	Store *Store
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

type itemReq struct {
{{- range .SubFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
}

type upsertReq struct {
{{- range .FormFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
{{- range .M2Ms}}
	{{exportedName .Name}}IDs []uint64 {{bq}}json:"{{.Name}}_ids"{{bq}}
{{- end}}
	Items []itemReq {{bq}}json:"items"{{bq}}
}

// toItems 把请求里的子表行转成模型；ID 与外键由 Store 在事务里统一回填。
func toItems(items []itemReq) []{{.SubEntity}} {
	out := make([]{{.SubEntity}}, 0, len(items))
	for _, it := range items {
		out = append(out, {{.SubEntity}}{
{{- range .SubFields}}
			{{.Column.GoField}}: it.{{.Column.GoField}},
{{- end}}
		})
	}
	return out
}

// List handles GET /api/v1/{{.Module}} —— 仅主表分页。
func (s *Server) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.List(ListFilter{
		Keyword: c.Query("keyword"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

// Detail handles GET /api/v1/{{.Module}}/:id —— 主表详情 + 全部子表行。
func (s *Server) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := s.Store.Get(id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	ok(c, m)
}

// Create handles POST /api/v1/{{.Module}} —— 主子表同事务创建。
func (s *Server) Create(c *gin.Context) {
	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	m := {{.Entity}}{
{{- range .FormFields}}
		{{.Column.GoField}}: req.{{.Column.GoField}},
{{- end}}
{{- range .M2Ms}}
		{{exportedName .Name}}IDs: req.{{exportedName .Name}}IDs,
{{- end}}
		Items: toItems(req.Items),
	}
	if err := s.Store.Create(&m); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, m)
}

// Update handles PUT /api/v1/{{.Module}}/:id —— 更新主表并全量替换子表行。
func (s *Server) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := s.Store.Get(id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
{{- range .FormFields}}
	m.{{.Column.GoField}} = req.{{.Column.GoField}}
{{- end}}
{{- range .M2Ms}}
	m.{{exportedName .Name}}IDs = req.{{exportedName .Name}}IDs
{{- end}}
	m.Items = toItems(req.Items)
	if err := s.Store.Update(m); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, m)
}

// Delete handles DELETE /api/v1/{{.Module}}/:id —— 连同子表行一起删除。
func (s *Server) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.Delete(id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"deleted": true})
}
`)

var tplSubRoutes = mustTpl("sub_routes", `package {{.Module}}

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts {{.Module}} master-detail CRUD routes. Wire auth
// middleware the same way as the sibling services (gateway X-Auth-* headers).
func (s *Server) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/v1/{{.Module}}", s.List)
	r.GET("/api/v1/{{.Module}}/:id", s.Detail)
	r.POST("/api/v1/{{.Module}}", s.Create)
	r.PUT("/api/v1/{{.Module}}/:id", s.Update)
	r.DELETE("/api/v1/{{.Module}}/:id", s.Delete)
}
`)

var tplSubAPI = mustTpl("sub_api", `import request from '@/utils/request'

export type {{.SubEntity}} = {
  id: number
  {{.SubFKCol.Name}}: number
{{- range .SubFields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
{{- if .SubHasAudit}}
  created_at: string
  updated_at: string
{{- end}}
}

export type {{.Entity}} = {
  id: number
{{- range .Fields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
{{- range .M2Ms}}
  {{.Name}}_ids?: number[]
{{- end}}
  created_at: string
  updated_at: string
  items?: {{.SubEntity}}[]
}

// 保存时子表行允许缺 id / 外键（新增行由后端在事务里补齐）
export type {{.Entity}}Upsert = Partial<Omit<{{.Entity}}, 'items'>> & { items?: Partial<{{.SubEntity}}>[] }

export type {{.Entity}}ListParams = {
  keyword?: string
  page: number
  page_size: number
}

export function list{{.Entity}}s(params: {{.Entity}}ListParams) {
  return request.get('/api/v1/{{.Module}}', { params }) as Promise<{ list: {{.Entity}}[]; total: number }>
}

export function get{{.Entity}}(id: number) {
  return request.get(`+"`/api/v1/{{.Module}}/${id}`"+`) as Promise<{{.Entity}}>
}

export function create{{.Entity}}(data: {{.Entity}}Upsert) {
  return request.post('/api/v1/{{.Module}}', data) as Promise<{{.Entity}}>
}

export function update{{.Entity}}(id: number, data: {{.Entity}}Upsert) {
  return request.put(`+"`/api/v1/{{.Module}}/${id}`"+`, data) as Promise<{{.Entity}}>
}

export function remove{{.Entity}}(id: number) {
  return request.delete(`+"`/api/v1/{{.Module}}/${id}`"+`) as Promise<{ deleted: boolean }>
}
`)

var tplSubPage = mustTplJSX("sub_page", `import { useEffect, useState } from 'react'
import {
  Button, Card, Form, Input, Modal, Popconfirm, Space, Table,
[[- if .HasNumberInput]]  InputNumber,
[[- end]][[- if .HasSwitchInput]]  Switch,
[[- end]][[- if .HasSelectInput]]  Select,
[[- end]]
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  create[[.Entity]], get[[.Entity]], list[[.Entity]]s, remove[[.Entity]], update[[.Entity]],
  type [[.Entity]], type [[.SubEntity]],
} from '@/api/[[.Module]]'
[[- if .DictTypes]]
import { getDictItems, type DictItem } from '@/api/dict'
[[- end]]
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

export default function [[.ModuleType]]Page() {
  const [list, setList] = useState<[[.Entity]][]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [editing, setEditing] = useState<[[.Entity]] | null>(null)
  const [creating, setCreating] = useState(false)
  // 子表行在弹窗里行内编辑，保存时随主表一起提交（全量替换）
  const [items, setItems] = useState<Partial<[[.SubEntity]]>[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
[[- if .DictTypes]]
  const [dictData, setDictData] = useState<Record<string, DictItem[]>>({})

  useEffect(() => {
    const loadDicts = async () => {
      const types = [
[[- range $i, $t := .DictTypes]]
[[- if $i]],
[[end]]        '[[.]]'
[[- end]]
      ]
      const results = await Promise.all(types.map(t => getDictItems(t).catch(() => [])))
      const map: Record<string, DictItem[]> = {}
      types.forEach((t, i) => { map[t] = results[i] })
      setDictData(map)
    }
    void loadDicts()
  }, [])

  const getDictLabel = (type: string, value: any): string => {
    const item = dictData[type]?.find(d => String(d.value) === String(value))
    return item?.label || String(value)
  }
[[- end]]
[[- if .M2Ms]]
  // 多对多关联目标表数据
[[- range .M2Ms]]
  const [ [[.Name]]Options, set[[exportedName .Name]]Options] = useState<{ label: string; value: number }[]>([])
[[- end]]

  useEffect(() => {
    const loadM2MTargets = async () => {
    }
    void loadM2MTargets()
  }, [])
[[- end]]

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await list[[.Entity]]s({ ...p })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error('获取数据失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchList(params)
  }, [params])

  async function openEditor(row: [[.Entity]] | null) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
[[- range .FormFields]]
      [[.Name]]: row?.[[.Name]][[if eq .Column.TSType "string"]] || ''[[end]],
[[- end]]
    })
    if (row) {
      try {
        const detail = await get[[.Entity]](row.id)
        setItems(detail.items ?? [])
      } catch {
        message.error('获取子表明细失败')
        setItems([])
      }
    } else {
      setItems([])
    }
  }

  function patchItem(idx: number, patch: Partial<[[.SubEntity]]>) {
    setItems((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  async function onSave() {
    const values = await form.validateFields()
    try {
      if (editing) await update[[.Entity]](editing.id, { ...values, items })
      else await create[[.Entity]]({ ...values, items })
      message.success('已保存')
      setEditing(null)
      setCreating(false)
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  async function onDelete(id: number) {
    try {
      await remove[[.Entity]](id)
      message.success('已删除')
      void fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  const columns: ColumnsType<[[.Entity]]> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
[[- range .ListFields]]
[[- if .DictType]]
    { title: '[[.Label]]', dataIndex: '[[.Name]]', render: (v: any) => getDictLabel('[[.DictType]]', v) },
[[- else if eq .Column.TSType "boolean"]]
    { title: '[[.Label]]', dataIndex: '[[.Name]]', render: (v: boolean) => (v ? '是' : '否') },
[[- else]]
    { title: '[[.Label]]', dataIndex: '[[.Name]]' },
[[- end]]
[[- end]]
    { title: '创建时间', dataIndex: 'created_at', width: 165, render: formatDateTime },
[[- range .M2Ms]]
    {
      title: '[[.Label]]', dataIndex: '[[.Name]]_ids', width: 200,
      render: (ids: number[]) => (
        <Space size={4} wrap>
          {ids?.map(id => {
            const opt = [[.Name]]Options.find(o => o.value === id)
            return opt ? <Tag key={id}>{opt.label}</Tag> : null
          })}
        </Space>
      ),
    },
[[- end]]
    {
      title: '操作', width: 140,
      render: (_, row) => (
        <Space size={0}>
          <Button type="link" size="small" onClick={() => void openEditor(row)}>编辑</Button>
          <Popconfirm title="确认删除？（子表行一并删除）" onConfirm={() => void onDelete(row.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // 子表行内可编辑列
  const itemColumns: ColumnsType<Partial<[[.SubEntity]]>> = [
[[- range .SubFields]]
[[- if eq .Column.TSType "number"]]
    {
      title: '[[.Label]]', dataIndex: '[[.Name]]',
      render: (_, r, idx) => (
        <InputNumber size="small" style={{ width: '100%' }} value={r.[[.Name]]}
          onChange={(v) => patchItem(idx, { [[.Name]]: v ?? 0 })} />
      ),
    },
[[- else if eq .Column.TSType "boolean"]]
    {
      title: '[[.Label]]', dataIndex: '[[.Name]]',
      render: (_, r, idx) => (
        <Switch size="small" checked={!!r.[[.Name]]} onChange={(v) => patchItem(idx, { [[.Name]]: v })} />
      ),
    },
[[- else]]
    {
      title: '[[.Label]]', dataIndex: '[[.Name]]',
      render: (_, r, idx) => (
        <Input size="small" value={r.[[.Name]] ?? ''} onChange={(e) => patchItem(idx, { [[.Name]]: e.target.value })} />
      ),
    },
[[- end]]
[[- end]]
    {
      title: '操作', width: 60,
      render: (_, __, idx) => (
        <Button type="link" size="small" danger icon={<DeleteOutlined />}
          onClick={() => setItems((prev) => prev.filter((_, i) => i !== idx))} />
      ),
    },
  ]

  return (
    <div className="page-list">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" initialValues={params}
          onFinish={(v) => setParams({ ...params, page: 1, ...v })}>
          <Form.Item name="keyword">
            <Input placeholder="关键字" prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.resetFields(); setParams({ page: 1, page_size: 10 }) }}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar title="[[.Title]]" total={total}
          extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => void openEditor(null)}>新建</Button>} />
        <Table rowKey="id" className="list-table" columns={columns} dataSource={list} loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无数据" compact /> }}
          pagination={{
            total, current: params.page, pageSize: params.page_size,
            showSizeChanger: true, showTotal: (t) => `+"`共 ${t} 条`"+`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }} />
      </Card>

      <Modal title={editing ? '编辑' : '新建'} open={creating || !!editing} width={860}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
[[- range .FormFields]]
[[- if .DictType]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <Select options={dictData['[[.DictType]]']?.map(d => ({ label: d.label, value: d.value }))} placeholder="请选择" allowClear />
          </Form.Item>
[[- else if eq .Column.TSType "number"]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <InputNumber style={{ width: '100%' }} />
          </Form.Item>
[[- else if eq .Column.TSType "boolean"]]
          <Form.Item name="[[.Name]]" label="[[.Label]]" valuePropName="checked">
            <Switch />
          </Form.Item>
[[- else]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <Input />
          </Form.Item>
[[- end]]
[[- end]]
[[- range .M2Ms]]
          <Form.Item name="[[.Name]]_ids" label="[[.Label]]">
            <Select mode="multiple" options={[[.Name]]Options} placeholder="请选择" allowClear showSearch />
          </Form.Item>
[[- end]]
        </Form>
        <div style={{ margin: '8px 0' }}>
          <strong>子表明细</strong>
          <span style={{ marginLeft: 8, color: '#999', fontSize: 12 }}>保存时按当前行全量替换</span>
        </div>
        <Table rowKey={(_, i) => i ?? 0} size="small" columns={itemColumns} dataSource={items}
          pagination={false} locale={{ emptyText: <GlassEmpty text="暂无明细" compact /> }} />
        <Button type="dashed" block style={{ marginTop: 8 }} icon={<PlusOutlined />}
          onClick={() => setItems((prev) => [...prev, {}])}>添加一行</Button>
      </Modal>
    </div>
  )
}
`)
