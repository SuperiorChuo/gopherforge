package system

// 树表模式模板。生成产物的约定照抄部门管理（identity-service 的
// department）：后端平铺查全量后递归组树、树接口返回整棵树、平铺分页
// 列表并存（搜索场景）、删除前检查子节点、前端嵌套 Table + TreeSelect
// 选父级（编辑时剪掉自己防成环）。单表模板见 codegen_templates.go。

var tplTreeModel = mustTpl("tree_model", `package {{.Module}}

import "time"

// {{.Entity}} maps table {{.Table}}（树表，{{.ParentCol.Name}} 自关联，0 表示根节点）.
type {{.Entity}} struct {
	ID uint64 {{bq}}gorm:"primaryKey" json:"id"{{bq}}
	{{.ParentCol.GoField}} uint64 {{bq}}gorm:"column:{{.ParentCol.Name}}" json:"{{.ParentCol.Name}}"{{bq}}
{{- range .Fields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}gorm:"column:{{.Name}}" json:"{{.Name}}"{{bq}}
{{- end}}
	CreatedAt time.Time {{bq}}json:"created_at"{{bq}}
	UpdatedAt time.Time {{bq}}json:"updated_at"{{bq}}
	// Children 不落库，树接口按 {{.ParentCol.Name}} 组树后返回（与部门树同款约定）
	Children []{{.Entity}} {{bq}}gorm:"-" json:"children,omitempty"{{bq}}
}

func ({{.Entity}}) TableName() string { return "{{.Table}}" }
`)

var tplTreeStore = mustTpl("tree_store", `package {{.Module}}

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// ErrHasChildren 表示节点仍有子节点，禁止删除（避免产生孤儿节点）。
var ErrHasChildren = errors.New("node has children")

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

// List 平铺分页列表（搜索场景用），树形展示走 Tree。
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
	err := q.Order("{{.TreeOrder}}").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// Tree 全量取出后在内存组树（与部门树同款做法）。
func (s *Store) Tree() ([]{{.Entity}}, error) {
	var all []{{.Entity}}
	if err := s.db.Order("{{.TreeOrder}}").Find(&all).Error; err != nil {
		return nil, err
	}
	return buildTree(all, 0), nil
}

func buildTree(items []{{.Entity}}, parentID uint64) []{{.Entity}} {
	var tree []{{.Entity}}
	for i := range items {
		if items[i].{{.ParentCol.GoField}} == parentID {
			children := buildTree(items, items[i].ID)
			if children == nil {
				items[i].Children = []{{.Entity}}{}
			} else {
				items[i].Children = children
			}
			tree = append(tree, items[i])
		}
	}
	return tree
}

func (s *Store) Get(id uint64) (*{{.Entity}}, error) {
	var m {{.Entity}}
	err := s.db.First(&m, id).Error
	return &m, err
}

func (s *Store) Create(m *{{.Entity}}) error { return s.db.Create(m).Error }

func (s *Store) Update(m *{{.Entity}}) error { return s.db.Save(m).Error }

// Delete 先查子节点，有子节点拒绝删除。
func (s *Store) Delete(id uint64) error {
	var count int64
	if err := s.db.Model(&{{.Entity}}{}).Where("{{.ParentCol.Name}} = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrHasChildren
	}
	return s.db.Delete(&{{.Entity}}{}, id).Error
}
`)

var tplTreeHandlers = mustTpl("tree_handlers", `package {{.Module}}

import (
	"errors"
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
	{{.ParentCol.GoField}} uint64 {{bq}}json:"{{.ParentCol.Name}}"{{bq}}
{{- range .FormFields}}
	{{.Column.GoField}} {{.Column.GoType}} {{bq}}json:"{{.Name}}"{{bq}}
{{- end}}
}

// List handles GET /api/v1/{{.Module}} —— 平铺分页（搜索场景）。
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

// Tree handles GET /api/v1/{{.Module}}/tree —— 整棵树（后端组树）。
func (s *Server) Tree(c *gin.Context) {
	tree, err := s.Store.Tree()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, tree)
}

// Create handles POST /api/v1/{{.Module}}.
func (s *Server) Create(c *gin.Context) {
	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.{{.ParentCol.GoField}} != 0 {
		if _, err := s.Store.Get(req.{{.ParentCol.GoField}}); err != nil {
			fail(c, http.StatusBadRequest, "父节点不存在")
			return
		}
	}
	m := {{.Entity}}{
		{{.ParentCol.GoField}}: req.{{.ParentCol.GoField}},
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
	// 不允许把自己设为父节点（成环保护，与部门管理同款）
	if req.{{.ParentCol.GoField}} == id {
		fail(c, http.StatusBadRequest, "不能选择自己作为父级")
		return
	}
	if req.{{.ParentCol.GoField}} != 0 {
		if _, err := s.Store.Get(req.{{.ParentCol.GoField}}); err != nil {
			fail(c, http.StatusBadRequest, "父节点不存在")
			return
		}
	}
	m.{{.ParentCol.GoField}} = req.{{.ParentCol.GoField}}
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
		if errors.Is(err, ErrHasChildren) {
			fail(c, http.StatusBadRequest, "该节点存在子节点，请先删除子节点")
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"deleted": true})
}
`)

var tplTreeRoutes = mustTpl("tree_routes", `package {{.Module}}

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts {{.Module}} tree CRUD routes. Wire auth middleware the
// same way as the sibling services (gateway X-Auth-* headers).
func (s *Server) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/v1/{{.Module}}", s.List)
	r.GET("/api/v1/{{.Module}}/tree", s.Tree)
	r.POST("/api/v1/{{.Module}}", s.Create)
	r.PUT("/api/v1/{{.Module}}/:id", s.Update)
	r.DELETE("/api/v1/{{.Module}}/:id", s.Delete)
}
`)

var tplTreeAPI = mustTpl("tree_api", `import request from '@/utils/request'

export type {{.Entity}} = {
  id: number
  {{.ParentCol.Name}}: number
{{- range .Fields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
  created_at: string
  updated_at: string
  children?: {{.Entity}}[]
}

export type {{.Entity}}ListParams = {
  keyword?: string
  page: number
  page_size: number
}

export function list{{.Entity}}s(params: {{.Entity}}ListParams) {
  return request.get('/api/v1/{{.Module}}', { params }) as Promise<{ list: {{.Entity}}[]; total: number }>
}

// 树接口：后端组好整棵树返回
export function get{{.Entity}}Tree() {
  return request.get('/api/v1/{{.Module}}/tree') as Promise<{{.Entity}}[]>
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

var tplTreePage = mustTplJSX("tree_page", `import { useEffect, useMemo, useState } from 'react'
import {
  Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Segmented, Space, Switch, Table, TreeSelect,
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  create[[.Entity]], get[[.Entity]]Tree, list[[.Entity]]s, remove[[.Entity]], update[[.Entity]], type [[.Entity]],
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

type TreeNode = { title: string; value: number; children?: TreeNode[] }

function toTreeSelectData(nodes: [[.Entity]][]): TreeNode[] {
  return nodes.map((n) => ({
    title: String(n.[[.NameField.Name]]),
    value: n.id,
    children: n.children?.length ? toTreeSelectData(n.children) : undefined,
  }))
}

function countTree(nodes: [[.Entity]][]): number {
  return nodes.reduce((acc, n) => acc + 1 + (n.children ? countTree(n.children) : 0), 0)
}

export default function [[.Entity]]Page() {
  const [list, setList] = useState<[[.Entity]][]>([])
  const [tree, setTree] = useState<[[.Entity]][]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<'tree' | 'list'>('tree')
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

  const fetchTree = async () => {
    setLoading(true)
    try {
      const res = await get[[.Entity]]Tree()
      setTree(res ?? [])
    } catch {
      message.error('获取树失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (view === 'list') void fetchList(params)
  }, [params, view])

  useEffect(() => {
    void fetchTree()
  }, [])

  const refresh = () => {
    void fetchTree()
    if (view === 'list') void fetchList(params)
  }

  // 编辑时禁止把自己选为上级（避免成环，与部门管理同款）
  const treeSelectData = useMemo(() => {
    const prune = (nodes: TreeNode[]): TreeNode[] =>
      nodes
        .filter((n) => n.value !== editing?.id)
        .map((n) => ({ ...n, children: n.children ? prune(n.children) : undefined }))
    return prune(toTreeSelectData(tree))
  }, [tree, editing])

  function openEditor(row: [[.Entity]] | null, parentId?: number) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
      [[.ParentCol.Name]]: row ? (row.[[.ParentCol.Name]] === 0 ? undefined : row.[[.ParentCol.Name]]) : parentId,
[[- range .FormFields]]
      [[.Name]]: row?.[[.Name]][[if eq .Column.TSType "string"]] || ''[[end]],
[[- end]]
    })
  }

  async function onSave() {
    const values = await form.validateFields()
    const payload = { ...values, [[.ParentCol.Name]]: values.[[.ParentCol.Name]] ?? 0 }
    try {
      if (editing) await update[[.Entity]](editing.id, payload)
      else await create[[.Entity]](payload)
      message.success('已保存')
      setEditing(null)
      setCreating(false)
      refresh()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  async function onDelete(id: number) {
    try {
      await remove[[.Entity]](id)
      message.success('已删除')
      refresh()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  const isTree = view === 'tree'

  const columns: ColumnsType<[[.Entity]]> = [
    { title: '[[.NameField.Label]]', dataIndex: '[[.NameField.Name]]' },
[[- range .TreeListFields]]
    { title: '[[.Label]]', dataIndex: '[[.Name]]'[[if eq .Column.TSType "boolean"]], render: (v: boolean) => (v ? '是' : '否')[[end]] },
[[- end]]
    { title: '创建时间', dataIndex: 'created_at', width: 165, render: formatDateTime },
    {
      title: '操作', width: 210,
      render: (_, row) => (
        <Space size={0}>
          <Button type="link" size="small" onClick={() => openEditor(null, row.id)}>新建下级</Button>
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
          onFinish={(v) => { setView('list'); setParams({ ...params, page: 1, ...v }) }}>
          <Form.Item name="keyword">
            <Input placeholder="关键字" prefix={<SearchOutlined />} allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.resetFields(); setParams({ page: 1, page_size: 10 }) }}>重置</Button>
            </Space>
          </Form.Item>
          <Form.Item style={{ marginInlineEnd: 0, marginLeft: 'auto' }}>
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'tree' | 'list')}
              options={[
                { label: '树形', value: 'tree' },
                { label: '列表', value: 'list' },
              ]}
            />
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar title="[[.Title]]" total={isTree ? countTree(tree) : total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={refresh}>刷新</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>新建</Button>
            </Space>
          } />
        <Table rowKey="id" className="list-table" columns={columns} dataSource={isTree ? tree : list} loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无数据" compact /> }}
          expandable={isTree ? { defaultExpandAllRows: true } : undefined}
          pagination={
            isTree
              ? false
              : {
                  total, current: params.page, pageSize: params.page_size,
                  showSizeChanger: true, showTotal: (t) => ` + "`共 ${t} 条`" + `,
                  onChange: (page, page_size) => setParams({ ...params, page, page_size }),
                }
          } />
      </Card>

      <Modal title={editing ? '编辑' : '新建'} open={creating || !!editing}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Form.Item name="[[.ParentCol.Name]]" label="上级节点">
            <TreeSelect
              treeData={treeSelectData}
              placeholder="不选则为顶级节点"
              allowClear
              showSearch
              treeDefaultExpandAll
              treeNodeFilterProp="title"
            />
          </Form.Item>
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
