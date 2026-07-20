import { useState } from 'react'
import {
  Button, Card, Checkbox, Form, Input, Modal, Space, Steps, Table, Tabs,
} from 'antd'
import { DownloadOutlined, EyeOutlined, ReloadOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  downloadCodegen, listCodegenColumns, listCodegenTables, previewCodegen,
  type CodegenColumn, type CodegenFieldConfig, type CodegenFile, type CodegenRequest, type CodegenTable,
} from '@/api/codegen'
import type { ColumnsType } from 'antd/es/table'
import GlassEmpty from '@/components/GlassEmpty'

export default function CodegenPage() {
  const [step, setStep] = useState(0)
  const [tables, setTables] = useState<CodegenTable[]>([])
  const [columns, setColumns] = useState<CodegenColumn[]>([])
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<{ table: string; module: string; title: string }>()
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
      form.setFieldsValue({ table, module: table.replace(/^[^a-z]+/, '').replace(/[^a-z0-9]/g, ''), title: table })
      setStep(1)
    } catch {
      message.error('加载字段失败')
    } finally {
      setLoading(false)
    }
  }

  function onNext() {
    form
      .validateFields()
      .then(() => setStep(2))
      .catch(() => {})
  }

  async function onPreview() {
    const values = await form.validateFields()
    setLoading(true)
    try {
      const req: CodegenRequest = { ...values, fields: fieldConfigs }
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
    const values = await form.validateFields()
    setLoading(true)
    try {
      const req: CodegenRequest = { ...values, fields: fieldConfigs }
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
            </Form>
            <h3 style={{ marginTop: 24, marginBottom: 16 }}>字段配置</h3>
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
