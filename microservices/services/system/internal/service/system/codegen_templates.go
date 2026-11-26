package system

// 代码生成器的模板。渲染出的产物遵循仓库实验线约定：后端使用
// gin + gorm + {code,message,data} 响应结构，前端采用管理端 Web 惯用写法
// （筛选卡片 + 表格 + 弹窗表单）。模板字面量有意包含中文界面文案；
// 它们随生成文件发布，而不出现在本服务的响应中。

import (
	"regexp"
	"strings"
	"text/template"
)

func mustRe(expr string) *regexp.Regexp { return regexp.MustCompile(expr) }

var tplFuncs = template.FuncMap{
	"bq":           func() string { return "`" },
	"exportedName": exportedName, // 使用 codegen.go 中已定义的函数
	"componentIs": func(component FormComponent, expected string) bool {
		return string(component) == expected
	},
	"sqlText": func(value string) string { return strings.ReplaceAll(value, "'", "''") },
}

func mustTpl(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(tplFuncs).Parse(body))
}

// mustTplJSX 使用 [[ ]] 分隔符，使 JSX 双花括号保持字面量。
func mustTplJSX(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(tplFuncs).Delims("[[", "]]").Parse(body))
}

var tplAPI = mustTpl("api", `import request from '@/utils/request'

export type {{.Entity}} = {
  id: number
{{- range .Fields}}
  {{.Name}}: {{.Column.TSType}}
{{- end}}
{{- range .M2Ms}}
  {{.Name}}_ids?: number[]
{{- end}}
  created_at?: string
  updated_at?: string
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

var tplPage = mustTplJSX("page", `import { useEffect, useState } from 'react'
import {
  Button, Card, Form, Input, Modal, Popconfirm, Space, Table,
[[- if .HasNumberInput]]  InputNumber,
[[- end]][[- if .HasSwitchInput]]  Switch,
[[- end]][[- if .HasSelectInput]]  Select,
[[- end]][[- if .HasTagList]]  Tag,
[[- end]][[- if .HasDateForm]]  DatePicker,
[[- end]]
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  create[[.Entity]], list[[.Entity]]s, remove[[.Entity]], update[[.Entity]], type [[.Entity]],
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
[[- if .HasDateForm]]
import dayjs from 'dayjs'
[[- end]]

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

  useEffect(() => {
    void fetchList(params)
  }, [params])

  function openEditor(row: [[.Entity]] | null) {
    setEditing(row)
    setCreating(!row)
    form.setFieldsValue({
[[- range .FormFields]]
      [[.Name]]: row?.[[.Name]][[if componentIs .Component "date"]] ? dayjs(row.[[.Name]]) : undefined[[else if componentIs .Component "datetime"]] ? dayjs(row.[[.Name]]) : undefined[[else if eq .Column.TSType "string"]] || ''[[end]],
[[- end]]
    })
  }

  async function onSave() {
    const values = await form.validateFields()
[[- if .HasDateForm]]
    const payload = {
      ...values,
[[- range .FormFields]]
[[- if componentIs .Component "date"]]      [[.Name]]: values.[[.Name]]?.format('YYYY-MM-DD'),
[[- else if componentIs .Component "datetime"]]      [[.Name]]: values.[[.Name]]?.toISOString(),
[[- end]]
[[- end]]
    }
[[- else]]
    const payload = values
[[- end]]
    try {
      if (editing) await update[[.Entity]](editing.id, payload)
      else await create[[.Entity]](payload)
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
            showSizeChanger: true, showTotal: (t) => `+"`共 ${t} 条`"+`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }} />
      </Card>

      <Modal title={editing ? '编辑' : '新建'} open={creating || !!editing}
        onCancel={() => { setEditing(null); setCreating(false) }} onOk={() => void onSave()} destroyOnHidden>
        <Form form={form} layout="vertical">
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
[[- else if or (componentIs .Component "date") (componentIs .Component "datetime")]]
          <Form.Item name="[[.Name]]" label="[[.Label]]"[[if .Required]] rules={[{ required: true }]}[[end]]>
            <DatePicker[[if componentIs .Component "datetime"]] showTime[[end]] style={{ width: '100%' }} />
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

