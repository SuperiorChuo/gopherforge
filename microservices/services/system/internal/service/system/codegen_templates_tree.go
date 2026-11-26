package system

// 树表模式模板。生成产物的约定照抄部门管理（identity-service 的
// department）：后端平铺查全量后递归组树、树接口返回整棵树、平铺分页
// 列表并存（搜索场景）、删除前检查子节点、前端嵌套 Table + TreeSelect
// 选父级（编辑时剪掉自己防成环）。单表模板见 codegen_templates.go。

var tplTreeAPI = mustTpl("tree_api", `import request from '@/utils/request'

export type {{.Entity}} = {
  id: number
  {{.ParentCol.Name}}: number
{{- range .Fields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
{{- range .M2Ms}}
  {{.Name}}_ids?: number[]
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
  return request.put(`+"`/api/v1/{{.Module}}/${id}`"+`, data) as Promise<{{.Entity}}>
}

export function remove{{.Entity}}(id: number) {
  return request.delete(`+"`/api/v1/{{.Module}}/${id}`"+`) as Promise<{ deleted: boolean }>
}

export type {{.ModuleType}}RelationOption = { label: string; value: number }

export function get{{.ModuleType}}RelationOptions(name: string) {
  return request.get(`+"`/api/v1/{{.Module}}/relations/${name}/options`"+`) as Promise<{{.ModuleType}}RelationOption[]>
}
`)

var tplTreePage = mustTplJSX("tree_page", `import { useEffect, useMemo, useState } from 'react'
import {
  Button, Card, Form, Input, Modal, Popconfirm, Segmented, Space, Table, TreeSelect,
[[- if .HasNumberInput]]  InputNumber,
[[- end]][[- if .HasSwitchInput]]  Switch,
[[- end]][[- if .HasSelectInput]]  Select,
[[- end]][[- if .HasTagList]]  Tag,
[[- end]]
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  create[[.Entity]], get[[.Entity]]Tree, list[[.Entity]]s, remove[[.Entity]], update[[.Entity]], type [[.Entity]],
[[- if .M2Ms]]  get[[.ModuleType]]RelationOptions,
[[- end]]
} from '@/api/[[.Module]]'
[[- if .DictTypes]]
import { getDictItems, type DictItem } from '@/api/dict'
[[- end]]
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
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

export default function [[.ModuleType]]Page() {
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
[[- range .M2Ms]]
      const [[.Name]]OptionsResult = await get[[$.ModuleType]]RelationOptions('[[.Name]]')
      set[[exportedName .Name]]Options([[.Name]]OptionsResult)
[[- end]]
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
                  showSizeChanger: true, showTotal: (t) => `+"`共 ${t} 条`"+`,
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
[[- if .DictType]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <Select options={dictData['[[.DictType]]']?.map(d => ({ label: d.label, value: d.value }))} placeholder="请选择" allowClear />
          </Form.Item>
[[- else if componentIs .Component "textarea"]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} />
          </Form.Item>
[[- else if componentIs .Component "number"]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <InputNumber style={{ width: '100%' }} />
          </Form.Item>
[[- else if componentIs .Component "switch"]]
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
      </Modal>
    </div>
  )
}
`)
