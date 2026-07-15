import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Space, Table, Tag, Typography, message as antMessage } from 'antd'
import { message } from '@/utils/feedback'
import request from '@/utils/request'

type SiteRow = {
  id: number
  app_key: string
  name: string
  welcome_text: string
  allowed_origins: string
  status: number
  snippet?: string
}

function parseOrigins(raw: string): string {
  try {
    const arr = JSON.parse(raw || '[]')
    if (Array.isArray(arr)) return arr.join('\n')
  } catch {
    /* ignore */
  }
  return raw || ''
}

function toOriginsJSON(text: string): string[] {
  return text
    .split(/\n|,/)
    .map((s) => s.trim())
    .filter(Boolean)
}

export default function ImSitesPage() {
  const [list, setList] = useState<SiteRow[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<SiteRow | null>(null)
  const [form] = Form.useForm()

  async function load() {
    setLoading(true)
    try {
      const data = (await request.get('/api/v1/im/admin/sites')) as { list: SiteRow[] }
      setList(data.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  function openEdit(row: SiteRow) {
    setEditing(row)
    form.setFieldsValue({
      name: row.name,
      welcome_text: row.welcome_text,
      allowed_origins: parseOrigins(row.allowed_origins),
    })
  }

  async function onSave() {
    if (!editing) return
    const values = await form.validateFields()
    try {
      await request.put(`/api/v1/im/admin/sites/${editing.id}`, {
        name: values.name,
        welcome_text: values.welcome_text,
        allowed_origins: toOriginsJSON(values.allowed_origins || ''),
      })
      message.success('已保存')
      setEditing(null)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  function copySnippet(snippet?: string) {
    if (!snippet) return
    void navigator.clipboard.writeText(snippet).then(
      () => antMessage.success('埋码已复制'),
      () => antMessage.error('复制失败，请手动选择'),
    )
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        title="智能客服 · 埋码站点 (IM M2)"
        extra={
          <Space>
            <Button href="/im/widget/demo" target="_blank">
              打开埋码演示页
            </Button>
            <Button href="/im/desk">坐席工作台</Button>
            <Button onClick={() => void load()}>刷新</Button>
          </Space>
        }
      >
        <Typography.Paragraph type="secondary">
          将「埋码」粘贴到客户网站 <code>&lt;/body&gt;</code> 前。聊天在 iframe 中打开，避免污染客户站样式。
          请把客户站域名加入「允许来源」。
        </Typography.Paragraph>
        <Table
          rowKey="id"
          loading={loading}
          dataSource={list}
          pagination={false}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 70 },
            { title: 'App Key', dataIndex: 'app_key', width: 120 },
            { title: '名称', dataIndex: 'name' },
            {
              title: '状态',
              dataIndex: 'status',
              width: 90,
              render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
            },
            {
              title: '操作',
              width: 220,
              render: (_, row) => (
                <Space>
                  <Button size="small" onClick={() => openEdit(row)}>
                    编辑
                  </Button>
                  <Button size="small" type="primary" onClick={() => copySnippet(row.snippet)}>
                    复制埋码
                  </Button>
                </Space>
              ),
            },
          ]}
          expandable={{
            expandedRowRender: (row) => (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{row.snippet}</pre>
            ),
          }}
        />
      </Card>

      {editing && (
        <Card title={`编辑站点 #${editing.id} (${editing.app_key})`} extra={<Button onClick={() => setEditing(null)}>取消</Button>}>
          <Form form={form} layout="vertical" onFinish={() => void onSave()}>
            <Form.Item name="name" label="名称" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="welcome_text" label="欢迎语">
              <Input.TextArea rows={3} />
            </Form.Item>
            <Form.Item
              name="allowed_origins"
              label="允许来源（每行一个 Origin，如 https://www.example.com）"
              extra="iframe 场景校验的是客户页 origin，不是网关域。"
            >
              <Input.TextArea rows={5} placeholder={'http://localhost:8000\nhttps://www.example.com'} />
            </Form.Item>
            <Button type="primary" htmlType="submit">
              保存
            </Button>
          </Form>
        </Card>
      )}
    </Space>
  )
}
