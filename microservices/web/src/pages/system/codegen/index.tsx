import { useState } from 'react'
import {
  Button, Card, Checkbox, Form, Input, Modal, Radio, Select, Space, Steps, Table, Tabs,
} from 'antd'
import { DownloadOutlined, EyeOutlined, ReloadOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  downloadCodegen, listCodegenColumns, listCodegenTables, previewCodegen,
  type CodegenColumn, type CodegenFieldConfig, type CodegenFile, type CodegenRequest,
  type CodegenTable, type CodegenTplType,
} from '@/api/codegen'
import type { ColumnsType } from 'antd/es/table'
import GlassEmpty from '@/components/GlassEmpty'

// 向导表单的全量值（step 切换会卸载 Form，值保留在 store 里，用 getFieldsValue(true) 读）
type WizardValues = {
  table?: string
  module?: string
  title?: string
  tpl_type?: CodegenTplType
  tree?: { parent_field?: string; name_field?: string; sort_field?: string }
  sub?: { table?: string; fk_field?: string }
}

export default function CodegenPage() {
  const [step, setStep] = useState(0)
  const [tables, setTables] = useState<CodegenTable[]>([])
  const [columns, setColumns] = useState<CodegenColumn[]>([])
  // 主子表模式：子表的列（选完子表后拉取，用于外键下拉）
  const [subColumns, setSubColumns] = useState<CodegenColumn[]>([])
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<WizardValues>()
  const tplType = (Form.useWatch('tpl_type', form) ?? 'crud') as CodegenTplType
  const [fieldConfigs, setFieldConfigs] = useState<CodegenFieldConfig[]>([])
  const [previewFiles, setPreviewFiles] = useState<CodegenFile[]>([])
  const [previewOpen, setPreviewOpen] = useState(false)

  async function loadTables() {
    setLoading(true)
    try {
      const res = await listCodegenTables()
      setTables(res.list ?? [])
    } catch {
      message.error('加载表列表失败')
    } finally {
      setLoading(false)
    }
  }

  async function onSelectTable(table: string) {
    setLoading(true)
    try {
      const res = await listCodegenColumns(table)
      const cols = res.list ?? []
      setColumns(cols)
      setFieldConfigs(
        cols
          .filter((c) => !['id', 'created_at', 'updated_at', 'deleted_at'].includes(c.name))
          .map((c) => ({
            name: c.name,
            label: c.label,
            in_list: true,
            in_search: c.go_type === 'string',
            in_form: true,
            required: false,
          })),
      )
      setSubColumns([])
      form.setFieldsValue({
        table,
        module: table.replace(/^[^a-z]+/, '').replace(/[^a-z0-9]/g, ''),
        title: table,
        tpl_type: 'crud',
        tree: { parent_field: undefined, name_field: undefined, sort_field: undefined },
        sub: { table: undefined, fk_field: undefined },
      })
      setStep(1)
    } catch {
      message.error('加载字段失败')
    } finally {
      setLoading(false)
    }
  }

  // 主子表模式：选中子表后拉取其列，供外键下拉选择
  async function onSelectSubTable(subTable: string) {
    form.setFieldsValue({ sub: { table: subTable, fk_field: undefined } })
    if (!subTable) {
      setSubColumns([])
      return
    }
    try {
      const res = await listCodegenColumns(subTable)
      const cols = res.list ?? []
      setSubColumns(cols)
      // 外键有明显候选（<主表单数>_id 或 主表名_id）时自动带出
      const main = form.getFieldValue('table') as string | undefined
      if (main) {
        const guesses = [`${main.replace(/s$/, '')}_id`, `${main}_id`]
        const hit = cols.find((c) => guesses.includes(c.name))
        if (hit) form.setFieldsValue({ sub: { table: subTable, fk_field: hit.name } })
      }
    } catch {
      message.error('加载子表字段失败')
    }
  }

  function onNext() {
    form
      .validateFields()
      .then(() => setStep(2))
      .catch(() => {})
  }

  // step 2 时第 1 步的 Form 已卸载，validateFields 拿不到值；从 store 全量读
  function collectRequest(): CodegenRequest | null {
    const values = form.getFieldsValue(true) as WizardValues
    if (!values.table || !values.module || !values.title) {
      message.error('配置不完整，请回上一步填写模块名与标题')
      setStep(1)
      return null
    }
    const tpl = values.tpl_type ?? 'crud'
    const req: CodegenRequest = {
      table: values.table,
      module: values.module,
      title: values.title,
      tpl_type: tpl,
      fields: fieldConfigs,
    }
    if (tpl === 'tree') {
      if (!values.tree?.parent_field || !values.tree?.name_field) {
        message.error('树表模式需要选择父级字段与显示字段')
        setStep(1)
        return null
      }
      req.tree = {
        parent_field: values.tree.parent_field,
        name_field: values.tree.name_field,
        sort_field: values.tree.sort_field || undefined,
      }
    }
    if (tpl === 'sub') {
      if (!values.sub?.table || !values.sub?.fk_field) {
        message.error('主子表模式需要选择子表与外键字段')
        setStep(1)
        return null
      }
      req.sub = { table: values.sub.table, fk_field: values.sub.fk_field }
    }
    return req
  }

  async function onPreview() {
    const req = collectRequest()
    if (!req) return
    setLoading(true)
    try {
      const res = await previewCodegen(req)
      setPreviewFiles(res.files ?? [])
      setPreviewOpen(true)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '预览失败')
    } finally {
      setLoading(false)
    }
  }

  async function onDownload() {
    const req = collectRequest()
    if (!req) return
    setLoading(true)
    try {
      const blob = await downloadCodegen(req)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `codegen-${req.module}.zip`
      a.click()
      URL.revokeObjectURL(url)
      message.success('已下载')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '下载失败')
    } finally {
      setLoading(false)
    }
  }

  const fieldColumns: ColumnsType<CodegenFieldConfig> = [
    { title: '字段', dataIndex: 'name', width: 150 },
    {
      title: '显示名',
      dataIndex: 'label',
      width: 150,
      render: (_, record, idx) => (
        <Input
          value={record.label}
          size="small"
          onChange={(e) => {
            const next = [...fieldConfigs]
            next[idx].label = e.target.value
            setFieldConfigs(next)
          }}
        />
      ),
    },
    {
      title: '列表显示',
      dataIndex: 'in_list',
      width: 90,
      render: (_, record, idx) => (
        <Checkbox
          checked={record.in_list}
          onChange={(e) => {
            const next = [...fieldConfigs]
            next[idx].in_list = e.target.checked
            setFieldConfigs(next)
          }}
        />
      ),
    },
    {
      title: '搜索',
      dataIndex: 'in_search',
      width: 80,
      render: (_, record, idx) => {
        const col = columns.find((c) => c.name === record.name)
        if (col?.go_type !== 'string') return <span style={{ color: '#999' }}>—</span>
        return (
          <Checkbox
            checked={record.in_search}
            onChange={(e) => {
              const next = [...fieldConfigs]
              next[idx].in_search = e.target.checked
              setFieldConfigs(next)
            }}
          />
        )
      },
    },
    {
      title: '表单',
      dataIndex: 'in_form',
      width: 80,
      render: (_, record, idx) => (
        <Checkbox
          checked={record.in_form}
          onChange={(e) => {
            const next = [...fieldConfigs]
            next[idx].in_form = e.target.checked
            setFieldConfigs(next)
          }}
        />
      ),
    },
    {
      title: '必填',
      dataIndex: 'required',
      width: 80,
      render: (_, record, idx) => (
        <Checkbox
          checked={record.required}
          onChange={(e) => {
            const next = [...fieldConfigs]
            next[idx].required = e.target.checked
            setFieldConfigs(next)
          }}
        />
      ),
    },
  ]

  return (
    <div className="page-detail">
      <Card bordered={false}>
        <Steps
          current={step}
          items={[
            { title: '选择表' },
            { title: '配置字段' },
            { title: '生成代码' },
          ]}
          style={{ marginBottom: 24 }}
        />

        {step === 0 && (
          <div>
            <Space style={{ marginBottom: 16 }}>
              <Button icon={<ReloadOutlined />} onClick={() => void loadTables()}>
                刷新表列表
              </Button>
            </Space>
            <Table
              rowKey="name"
              size="small"
              columns={[
                { title: '表名', dataIndex: 'name' },
                {
                  title: '操作',
                  width: 120,
                  render: (_, row) => (
                    <Button type="link" size="small" onClick={() => void onSelectTable(row.name)}>
                      选择
                    </Button>
                  ),
                },
              ]}
              dataSource={tables}
              loading={loading}
              locale={{ emptyText: <GlassEmpty text="点击刷新加载表列表" compact /> }}
              pagination={{ pageSize: 20, showSizeChanger: false }}
            />
          </div>
        )}

        {step === 1 && (
          <div>
            <Form form={form} layout="vertical">
              <Form.Item name="table" label="数据表" rules={[{ required: true }]}>
                <Input disabled />
              </Form.Item>
              <Form.Item
                name="module"
                label="模块名（小写英文，作为路由和文件夹）"
                rules={[{ required: true, pattern: /^[a-z][a-z0-9]{1,31}$/, message: '小写字母开头，2-32字符' }]}
              >
                <Input placeholder="例: assets" />
              </Form.Item>
              <Form.Item name="title" label="页面标题" rules={[{ required: true }]}>
                <Input placeholder="例: 资产管理" />
              </Form.Item>
              <Form.Item name="tpl_type" label="生成模式" initialValue="crud">
                <Radio.Group
                  options={[
                    { label: '单表（标准 CRUD）', value: 'crud' },
                    { label: '树表（父子层级）', value: 'tree' },
                    { label: '主子表（一对多明细）', value: 'sub' },
                  ]}
                />
              </Form.Item>
              {tplType === 'tree' && (
                <>
                  <Form.Item
                    name={['tree', 'parent_field']}
                    label="父级字段（自关联，指向本表 id，如 parent_id）"
                    rules={[{ required: true, message: '请选择父级字段' }]}
                  >
                    <Select
                      placeholder="选择整数类型字段"
                      options={columns
                        .filter((c) => !c.primary_key && c.go_type === 'int64' && !['created_at', 'updated_at', 'deleted_at'].includes(c.name))
                        .map((c) => ({ label: `${c.name} (${c.db_type})`, value: c.name }))}
                    />
                  </Form.Item>
                  <Form.Item
                    name={['tree', 'name_field']}
                    label="显示字段（树节点标题，如 name）"
                    rules={[{ required: true, message: '请选择显示字段' }]}
                  >
                    <Select
                      placeholder="选择文本类型字段"
                      options={columns
                        .filter((c) => !c.primary_key && c.go_type === 'string' && !['created_at', 'updated_at', 'deleted_at'].includes(c.name))
                        .map((c) => ({ label: `${c.name} (${c.db_type})`, value: c.name }))}
                    />
                  </Form.Item>
                  <Form.Item name={['tree', 'sort_field']} label="排序字段（可选，如 sort）">
                    <Select
                      allowClear
                      placeholder="不选则按 id 排序"
                      options={columns
                        .filter((c) => !c.primary_key && !['created_at', 'updated_at', 'deleted_at'].includes(c.name))
                        .map((c) => ({ label: `${c.name} (${c.db_type})`, value: c.name }))}
                    />
                  </Form.Item>
                </>
              )}
              {tplType === 'sub' && (
                <>
                  <Form.Item
                    name={['sub', 'table']}
                    label="子表（明细表，与主表一对多）"
                    rules={[{ required: true, message: '请选择子表' }]}
                  >
                    <Select
                      showSearch
                      placeholder="选择子表"
                      onChange={(v) => void onSelectSubTable(v as string)}
                      options={tables
                        .filter((tb) => tb.name !== form.getFieldValue('table'))
                        .map((tb) => ({ label: tb.name, value: tb.name }))}
                    />
                  </Form.Item>
                  <Form.Item
                    name={['sub', 'fk_field']}
                    label="子表外键字段（指向主表 id）"
                    rules={[{ required: true, message: '请选择外键字段' }]}
                  >
                    <Select
                      placeholder="先选子表"
                      options={subColumns
                        .filter((c) => !c.primary_key && c.go_type === 'int64')
                        .map((c) => ({ label: `${c.name} (${c.db_type})`, value: c.name }))}
                    />
                  </Form.Item>
                </>
              )}
            </Form>
            {tplType === 'sub' && (
              <p style={{ color: '#999', fontSize: 12, marginBottom: 16 }}>
                子表字段自动全量生成（去掉主键、外键与审计列）；保存主表时子表行在同一事务内全量替换。
              </p>
            )}
            <h3 style={{ marginTop: 24, marginBottom: 16 }}>字段配置{tplType === 'sub' ? '（主表）' : ''}</h3>
            <Table
              rowKey="name"
              size="small"
              columns={fieldColumns}
              dataSource={fieldConfigs}
              pagination={false}
            />
            <Space style={{ marginTop: 24 }}>
              <Button onClick={() => setStep(0)}>上一步</Button>
              <Button type="primary" onClick={onNext}>
                下一步
              </Button>
            </Space>
          </div>
        )}

        {step === 2 && (
          <div>
            <p style={{ marginBottom: 16 }}>
              点击<strong>预览</strong>查看生成的代码，或直接<strong>下载</strong>为 zip 压缩包。
            </p>
            <Space>
              <Button icon={<EyeOutlined />} onClick={() => void onPreview()} loading={loading}>
                预览代码
              </Button>
              <Button
                type="primary"
                icon={<DownloadOutlined />}
                onClick={() => void onDownload()}
                loading={loading}
              >
                下载代码包
              </Button>
              <Button onClick={() => setStep(1)}>上一步</Button>
              <Button
                onClick={() => {
                  setStep(0)
                  setColumns([])
                  setSubColumns([])
                  setFieldConfigs([])
                  form.resetFields()
                }}
              >
                重新生成
              </Button>
            </Space>
          </div>
        )}
      </Card>

      <Modal
        title="代码预览"
        open={previewOpen}
        onCancel={() => setPreviewOpen(false)}
        footer={null}
        width="80vw"
        style={{ top: 40 }}
      >
        <Tabs
          items={previewFiles.map((f) => ({
            key: f.path,
            label: f.path,
            children: (
              <pre
                style={{
                  maxHeight: '60vh',
                  overflow: 'auto',
                  background: '#f6f6f6',
                  padding: 12,
                  borderRadius: 4,
                  fontSize: 13,
                  lineHeight: 1.5,
                }}
              >
                {f.content}
              </pre>
            ),
          }))}
        />
      </Modal>
    </div>
  )
}
