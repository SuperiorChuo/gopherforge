import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Checkbox, Form, Input, Space, Switch, Table, TimePicker, Typography, Grid } from 'antd'
import { ReloadOutlined, GlobalOutlined, EditOutlined, CopyOutlined, PlusOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import { message } from '@/utils/feedback'
import { copyToClipboard } from '@/utils/clipboard'
import request from '@/utils/request'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { useTableQuery } from '@/hooks/useTableQuery'

type SiteRow = {
  id: number
  app_key: string
  name: string
  welcome_text: string
  allowed_origins: string
  status: number
  bot_enabled?: boolean
  bot_kb_enabled?: boolean
  bot_system_prompt?: string
  business_hours?: string
  snippet?: string
}

type BizHours = {
  enabled: boolean
  days: number[]
  start: string
  end: string
  off_text?: string
}

const DAY_OPTIONS = [
  { label: '一', value: 1 }, { label: '二', value: 2 }, { label: '三', value: 3 },
  { label: '四', value: 4 }, { label: '五', value: 5 }, { label: '六', value: 6 }, { label: '日', value: 7 },
]

function bizHoursToForm(bh: BizHours) {
  return {
    bh_enabled: bh.enabled,
    bh_days: bh.days,
    bh_range: [dayjs(bh.start, 'HH:mm'), dayjs(bh.end, 'HH:mm')],
    bh_off_text: bh.off_text || '',
  }
}

function parseBizHours(raw?: string): BizHours {
  try {
    const o = JSON.parse(raw || '{}')
    return {
      enabled: !!o.enabled,
      days: Array.isArray(o.days) && o.days.length ? o.days : [1, 2, 3, 4, 5],
      start: o.start || '09:00',
      end: o.end || '18:00',
      off_text: o.off_text || '',
    }
  } catch {
    return { enabled: false, days: [1, 2, 3, 4, 5], start: '09:00', end: '18:00', off_text: '' }
  }
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
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  // editing=行 → 编辑；creating → 新增；二者共用同一张表单卡
  const [editing, setEditing] = useState<SiteRow | null>(null)
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm()

  const fetcher = useCallback(async () => {
    const data = (await request.get('/api/v1/im/admin/sites')) as { list: SiteRow[] }
    const list = data.list || []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('加载失败')), [t]),
  })

  function openEdit(row: SiteRow) {
    setCreating(false)
    setEditing(row)
    form.setFieldsValue({
      name: row.name,
      welcome_text: row.welcome_text,
      allowed_origins: parseOrigins(row.allowed_origins),
      bot_enabled: row.bot_enabled !== false,
      bot_kb_enabled: row.bot_kb_enabled !== false,
      bot_system_prompt: row.bot_system_prompt || '',
      ...bizHoursToForm(parseBizHours(row.business_hours)),
    })
  }

  function openCreate() {
    setEditing(null)
    setCreating(true)
    form.setFieldsValue({
      name: '',
      welcome_text: '',
      allowed_origins: '',
      bot_enabled: true,
      bot_kb_enabled: true,
      bot_system_prompt: '',
      ...bizHoursToForm(parseBizHours()),
    })
  }

  function closeEditor() {
    setEditing(null)
    setCreating(false)
  }

  async function onSave() {
    if (!editing && !creating) return
    const values = await form.validateFields()
    const bh: BizHours = {
      enabled: !!values.bh_enabled,
      days: values.bh_days || [],
      start: values.bh_range?.[0] ? (values.bh_range[0] as Dayjs).format('HH:mm') : '09:00',
      end: values.bh_range?.[1] ? (values.bh_range[1] as Dayjs).format('HH:mm') : '18:00',
      off_text: values.bh_off_text || '',
    }
    const body = {
      name: values.name,
      welcome_text: values.welcome_text,
      allowed_origins: toOriginsJSON(values.allowed_origins || ''),
      bot_enabled: !!values.bot_enabled,
      bot_kb_enabled: !!values.bot_kb_enabled,
      bot_system_prompt: values.bot_system_prompt || '',
      business_hours: bh.enabled ? JSON.stringify(bh) : '',
    }
    try {
      if (creating) {
        await request.post('/api/v1/im/admin/sites', body)
        message.success(t('站点已创建，复制埋码即可接入'))
      } else {
        await request.put(`/api/v1/im/admin/sites/${editing!.id}`, body)
        message.success(t('已保存'))
      }
      closeEditor()
      await reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  function copySnippet(snippet?: string) {
    if (!snippet) return
    void copyToClipboard(snippet).then(
      (ok) => (ok ? message.success(t('埋码已复制')) : message.error(t('复制失败，请手动选择'))),
    )
  }

  return (
    <ListPageShell
      className="im-sites-page"
      toolbar={
        <TableToolbar
          title="埋码站点"
          total={list.length}
          icon={<GlobalOutlined />}
          gradient="linear-gradient(135deg, #22d3ee, #0891b2)"
          glow="rgba(8, 145, 178, 0.4)"
          description="客服组件埋码接入的站点与来源白名单"
          extra={
            <Space wrap>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('新增站点')}
              </Button>
              <Button href="/im/widget/demo.html" target="_blank">
                {t('打开埋码演示页')}
              </Button>
              <Button href="/im/desk">{t('坐席工作台')}</Button>
              <Button href="/im/skills">{t('技能组')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Typography.Paragraph type="secondary">
          {t('将「埋码」粘贴到客户网站')}<code>&lt;/body&gt;</code>{t('前。聊天在 iframe 中打开，避免污染客户站样式。请把客户站域名加入「允许来源」。')}
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
            { title: t('名称'), dataIndex: 'name' },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 100,
              render: (v: number) =>
                v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
            },
            {
              title: t('机器人'),
              dataIndex: 'bot_enabled',
              width: 100,
              render: (v: boolean) =>
                v ? <StatusPill tone="info" label="已开启" pulse={false} /> : <StatusPill tone="muted" label="关闭" />,
            },
            {
              title: t('操作'),
              width: compactActions ? 48 : 88,
              align: 'center' as const,
              fixed: 'right' as const,
              render: (_, row) => (
                <TableRowActions
                  menuOnly={compactActions}
                  maxInline={2}
                  ariaLabel={t('更多操作：{{name}}', { name: row.name })}
                  actions={[
                    { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(row) },
                    { key: 'copy', label: t('复制埋码'), icon: <CopyOutlined />, onClick: () => copySnippet(row.snippet) },
                  ]}
                />
              ),
            },
          ]}
          expandable={{
            expandedRowRender: (row) => (
              <pre className="cell-mono" style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{row.snippet}</pre>
            ),
          }}
         scroll={{ x: 960 }} />

      {(creating || editing) && (
        <Card
          className="list-main-card"
          bordered={false}
          title={creating ? t('新增站点（App Key 自动生成）') : t('编辑站点 #{{id}} ({{app_key}})', { id: editing!.id, app_key: editing!.app_key })}
          extra={<Button onClick={closeEditor}>{t('取消')}</Button>}
        >
          <Form form={form} layout="vertical" onFinish={() => void onSave()}>
            <Form.Item name="name" label={t('名称')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="welcome_text" label={t('欢迎语')}>
              <Input.TextArea rows={3} />
            </Form.Item>
            <Form.Item name="bot_enabled" label={t('启用机器人预答')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item
              name="bot_kb_enabled"
              label={t('知识库预答')}
              valuePropName="checked"
              extra={t('开启后机器人优先检索 AI 知识库，未命中会记入「待补充知识」')}
            >
              <Switch checkedChildren={t('开')} unCheckedChildren={t('关')} />
            </Form.Item>
            <Form.Item name="bot_system_prompt" label={t('机器人 System Prompt（可选）')} extra={t('留空则用服务默认提示词')}>
              <Input.TextArea rows={3} placeholder={t('你是企业在线客服助手…')} />
            </Form.Item>
            <Form.Item
              name="allowed_origins"
              label={t('允许来源（每行一个 Origin，如 https://www.example.com）')}
              extra={t('iframe 场景校验的是客户页 origin，不是网关域。')}
            >
              <Input.TextArea rows={5} placeholder={'http://localhost:8000\nhttps://www.example.com'} />
            </Form.Item>
            <Form.Item name="bh_enabled" label={t('启用营业时间')} valuePropName="checked"
              extra={t('非营业时间访客仍可留言，会自动收到下方提示语；支持跨夜时段（如 22:00-06:00）。')}>
              <Switch checkedChildren={t('开')} unCheckedChildren={t('关')} />
            </Form.Item>
            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.bh_enabled !== cur.bh_enabled}>
              {({ getFieldValue }) =>
                getFieldValue('bh_enabled') ? (
                  <>
                    <Form.Item name="bh_days" label={t('工作日')}>
                      <Checkbox.Group options={DAY_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} />
                    </Form.Item>
                    <Form.Item name="bh_range" label={t('工作时段')}>
                      <TimePicker.RangePicker format="HH:mm" minuteStep={15} order={false} />
                    </Form.Item>
                    <Form.Item name="bh_off_text" label={t('非工作时间提示语')}>
                      <Input placeholder={t('当前为非工作时间，您可以留言，我们上班后会尽快回复。')} />
                    </Form.Item>
                  </>
                ) : null
              }
            </Form.Item>
            <Button type="primary" htmlType="submit">
              {t('保存')}
            </Button>
          </Form>
        </Card>
      )}
    </ListPageShell>
  )
}
