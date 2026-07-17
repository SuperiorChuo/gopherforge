import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Space, Switch, Table, Typography } from 'antd'
import { ReloadOutlined, GlobalOutlined, EditOutlined, CopyOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import request from '@/utils/request'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'

type SiteRow = {
  id: number
  app_key: string
  name: string
  welcome_text: string
  allowed_origins: string
  status: number
  bot_enabled?: boolean
  bot_system_prompt?: string
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
      bot_enabled: row.bot_enabled !== false,
      bot_system_prompt: row.bot_system_prompt || '',
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
        bot_enabled: !!values.bot_enabled,
        bot_system_prompt: values.bot_system_prompt || '',
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
      () => message.success('埋码已复制'),
      () => message.error('复制失败，请手动选择'),
    )
  }

  return (
    <div className="page-list im-sites-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="埋码站点"
          total={list.length}
          icon={<GlobalOutlined />}
          gradient="linear-gradient(135deg, #22d3ee, #0891b2)"
          glow="rgba(8, 145, 178, 0.4)"
          description="客服组件埋码接入的站点与来源白名单"
          extra={
            <Space wrap>
              <Button href="/im/widget/demo.html" target="_blank">
                打开埋码演示页
              </Button>
              <Button href="/im/desk">坐席工作台</Button>
              <Button href="/im/skills">技能组</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void load()}>刷新</Button>
            </Space>
          }
        />
        <Typography.Paragraph type="secondary">
          将「埋码」粘贴到客户网站 <code>&lt;/body&gt;</code> 前。聊天在 iframe 中打开，避免污染客户站样式。
          请把客户站域名加入「允许来源」。
        </Typography.Paragraph>
        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          dataSource={list}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无接入站点" compact /> }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 70 },
            {
              title: 'App Key',
              dataIndex: 'app_key',
              width: 140,
              render: (v: string) => <span className="cell-mono">{v}</span>,
            },
            { title: '名称', dataIndex: 'name' },
            {
              title: '状态',
              dataIndex: 'status',
              width: 100,
              render: (v: number) =>
                v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
            },
            {
              title: '机器人',
              dataIndex: 'bot_enabled',
              width: 100,
              render: (v: boolean) =>
                v ? <StatusPill tone="info" label="已开启" pulse={false} /> : <StatusPill tone="muted" label="关闭" />,
            },
            {
              title: '操作',
              width: 220,
              render: (_, row) => (
                <Space size={0} className="table-actions">
                  <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(row)}>
                    编辑
                  </Button>
                  <Button type="link" size="small" icon={<CopyOutlined />} onClick={() => copySnippet(row.snippet)}>
                    复制埋码
                  </Button>
                </Space>
              ),
            },
          ]}
          expandable={{
            expandedRowRender: (row) => (
              <pre className="cell-mono" style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{row.snippet}</pre>
            ),
          }}
        />
      </Card>

      {editing && (
        <Card
          className="list-main-card"
          bordered={false}
          title={`编辑站点 #${editing.id} (${editing.app_key})`}
          extra={<Button onClick={() => setEditing(null)}>取消</Button>}
        >
          <Form form={form} layout="vertical" onFinish={() => void onSave()}>
            <Form.Item name="name" label="名称" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="welcome_text" label="欢迎语">
              <Input.TextArea rows={3} />
            </Form.Item>
            <Form.Item name="bot_enabled" label="启用机器人预答" valuePropName="checked">
              <Switch checkedChildren="开" unCheckedChildren="关" />
            </Form.Item>
            <Form.Item name="bot_system_prompt" label="机器人 System Prompt（可选）" extra="留空则用服务默认提示词">
              <Input.TextArea rows={3} placeholder="你是企业在线客服助手…" />
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
    </div>
  )
}
