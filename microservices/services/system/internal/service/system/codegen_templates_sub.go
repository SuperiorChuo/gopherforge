package system

// 主子表模式模板。语义借鉴 ruoyi-vue-pro：主表 CRUD + 详情带子表行、
// 创建/更新在同一事务里全量替换子表行（先删后插）、删除连同子表行。
// 子表字段由数据库自动推导（去掉主键/外键/审计列），前端在弹窗里用
// 行内可编辑表格增删子表行。单表模板见 codegen_templates.go。

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
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
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
