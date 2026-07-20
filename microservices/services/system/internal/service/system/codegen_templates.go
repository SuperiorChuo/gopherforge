package system

// Templates for the code generator. Rendered artifacts follow the repo's
// experimental-line conventions (gin + gorm + {code,message,data} envelope)
// for the backend, and the admin web idiom (filter card + table + modal
// form) for the frontend. Template literals contain Chinese UI copy by
// design; it ships in generated files, not in this service's responses.

import (
	"regexp"
	"text/template"
)

func mustRe(expr string) *regexp.Regexp { return regexp.MustCompile(expr) }

var tplFuncs = template.FuncMap{
	"bq": func() string { return "`" },
}

func mustTpl(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(tplFuncs).Parse(body))
}

// mustTplJSX uses [[ ]] delimiters so JSX double braces stay literal.
func mustTplJSX(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(tplFuncs).Delims("[[", "]]").Parse(body))
}

var tplModel = mustTpl("model", `package {{.Module}}

import "time"

// {{.Entity}} maps table {{.Table}}.
type {{.Entity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
{{- range .Fields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
}

func ({{.Entity}}) TableName() string { return "{{.Table}}" }
`)

var tplStore = mustTpl("store", `package {{.Module}}

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
	return list, total, err
}

func (s *Store) Get(id uint64) (*{{.Entity}}, error) {
	var m {{.Entity}}
	err := s.db.First(&m, id).Error
	return &m, err
}

func (s *Store) Create(m *{{.Entity}}) error { return s.db.Create(m).Error }

func (s *Store) Update(m *{{.Entity}}) error { return s.db.Save(m).Error }

func (s *Store) Delete(id uint64) error {
	return s.db.Delete(&{{.Entity}}{}, id).Error
}
`)

var tplHandlers = mustTpl("handlers", `package {{.Module}}

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

type upsertReq struct {
{{- range .FormFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
}

// List handles GET /api/v1/{{.Module}}.
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

// Create handles POST /api/v1/{{.Module}}.
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
	}
	if err := s.Store.Create(&m); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, m)
}

// Update handles PUT /api/v1/{{.Module}}/:id.
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
	if err := s.Store.Update(m); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, m)
}

// Delete handles DELETE /api/v1/{{.Module}}/:id.
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

var tplRoutes = mustTpl("routes", `package {{.Module}}

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts {{.Module}} CRUD routes. Wire auth middleware the
// same way as the sibling services (gateway X-Auth-* headers).
func (s *Server) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/v1/{{.Module}}", s.List)
	r.POST("/api/v1/{{.Module}}", s.Create)
	r.PUT("/api/v1/{{.Module}}/:id", s.Update)
	r.DELETE("/api/v1/{{.Module}}/:id", s.Delete)
}
`)

var tplAPI = mustTpl("api", `import request from '@/utils/request'

export type {{.Entity}} = {
  id: number
{{- range .Fields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
  created_at: string
  updated_at: string
}

export type {{.Entity}}ListParams = {
  keyword?: string
  page: number
  page_size: number
}

export function list{{.Entity}}s(params: {{.Entity}}ListParams) {
  return request.get('/api/v1/{{.Module}}', { params }) as Promise<{ list: {{.Entity}}[]; total: number }>
}

export function create{{.Entity}}(data: Partial<{{.Entity}}>) {
  return request.post('/api/v1/{{.Module}}', data) as Promise<{{.Entity}}>
}

export function update{{.Entity}}(id: number, data: Partial<{{.Entity}}>) {
  return request.put(` + "`/api/v1/{{.Module}}/${id}`" + `, data) as Promise<{{.Entity}}>
}

export function remove{{.Entity}}(id: number) {
  return request.delete(` + "`/api/v1/{{.Module}}/${id}`" + `) as Promise<{ deleted: boolean }>
}
`)

var tplPage = mustTplJSX("page", `import { useEffect, useState } from 'react'
import {
  Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Space, Switch, Table,
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  create[[.Entity]], list[[.Entity]]s, remove[[.Entity]], update[[.Entity]], type [[.Entity]],
} from '@/api/[[.Module]]'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

export default function [[.Entity]]Page() {
  const [list, setList] = useState<[[.Entity]][]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [editing, setEditing] = useState<[[.Entity]] | null>(null)
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()

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

  function openEditor(row: [[.Entity]] | null) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
[[- range .FormFields]]
      [[.Name]]: row?.[[.Name]][[if eq .Column.TSType "string"]] || ''[[end]],
[[- end]]
    })
  }

  async function onSave() {
    const values = await form.validateFields()
    try {
      if (editing) await update[[.Entity]](editing.id, values)
      else await create[[.Entity]](values)
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
    { title: '[[.Label]]', dataIndex: '[[.Name]]'[[if eq .Column.TSType "boolean"]], render: (v: boolean) => (v ? '是' : '否')[[end]] },
[[- end]]
    { title: '创建时间', dataIndex: 'created_at', width: 165, render: formatDateTime },
    {
      title: '操作', width: 140,
      render: (_, row) => (
        <Space size={0}>
          <Button type="link" size="small" onClick={() => openEditor(row)}>编辑</Button>
          <Popconfirm title="确认删除？" onConfirm={() => void onDelete(row.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
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
          extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>新建</Button>} />
        <Table rowKey="id" className="list-table" columns={columns} dataSource={list} loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无数据" compact /> }}
          pagination={{
            total, current: params.page, pageSize: params.page_size,
            showSizeChanger: true, showTotal: (t) => ` + "`共 ${t} 条`" + `,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }} />
      </Card>

      <Modal title={editing ? '编辑' : '新建'} open={creating || !!editing}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
[[- range .FormFields]]
[[- if eq .Column.TSType "number"]]
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
        </Form>
      </Modal>
    </div>
  )
}
`)

var tplMenu = mustTpl("menu", `-- Menu seed for {{.Title}} (adjust id/parent_id/sort to your tree)
INSERT INTO menus (name, title, icon, path, component, parent_id, sort, status, hidden, created_at, updated_at)
VALUES ('{{.Module}}', '{{.Title}}', 'file', '/{{.Module}}', '{{.Module}}/index', 0, 50, 1, 0, NOW(), NOW());
`)
