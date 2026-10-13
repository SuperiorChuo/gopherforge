import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Card, Checkbox, Drawer, Form, Input, Select, Space, Switch, Table, Tabs, Tag,
  Tooltip, Typography, Alert,
} from 'antd'
import {
  NotificationOutlined, ReloadOutlined, SendOutlined, EyeOutlined, EditOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  getNotifyChannels,
  listAllNotifyDeliveries,
  listNotifyTemplates,
  previewNotifyTemplate,
  testSendNotifyTemplate,
  updateNotifyTemplate,
  type NotifyChannelStatus,
  type NotifyDeliveryAdmin,
  type NotifyTemplate,
} from '@/api/notify'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { message } from '@/utils/feedback'
import { useAppSelector } from '@/hooks/store'
import { displayUserName, useUserNameMap } from '@/hooks/useUserNameMap'
import { formatDateTime } from '@/utils/format'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import './styles.css'

const CHANNEL_OPTS = [
  { label: '站内信', value: 'inbox' },
  { label: '邮件', value: 'email' },
  { label: '短信', value: 'sms' },
  { label: '语音', value: 'voice' },
]

const CODE_LABEL: Record<string, string> = {
  'ticket.overdue': '工单超时',
  'ticket.assigned': '工单指派',
  'ticket.comment': '工单评论',
  'ticket.sla_warning': 'SLA 预警',
  'im.transfer_human': 'IM 转人工',
  'im.low_rating': 'IM 低分',
  'crm.followup_due': 'CRM 回访到期',
  'crm.customer_recycled': '公海回收',
}

export default function NotifyTemplatesPage() {
  const { t } = useTranslation()
  const isPlatform = !!useAppSelector((s) => s.auth.userInfo)?.is_platform_admin
  const [channels, setChannels] = useState<NotifyChannelStatus | null>(null)
  const editModal = useCrudModal<NotifyTemplate>()
  const [saving, setSaving] = useState(false)
  const [preview, setPreview] = useState<{ title: string; content: string; link: string; channels: string[] } | null>(null)
  const [form] = Form.useForm()

  const fetcher = useCallback(async () => {
    const [tplRes, ch] = await Promise.all([listNotifyTemplates(), getNotifyChannels()])
    setChannels(tplRes?.channels ?? ch ?? null)
    const list = tplRes?.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('加载失败')), [t]),
  })

  const openEdit = (row: NotifyTemplate) => {
    editModal.openEdit(row)
    setPreview(null)
    form.setFieldsValue({
      title_tpl: row.title_tpl,
      body_tpl: row.body_tpl,
      link_tpl: row.link_tpl,
      channels: (row.channels || row.channel || 'inbox').split(',').map((s) => s.trim()).filter(Boolean),
      enabled: row.enabled,
    })
  }

  const handleSave = async () => {
    if (!editModal.record) return
    const v = await form.validateFields().catch(() => null)
    if (!v) return
    setSaving(true)
    try {
      await updateNotifyTemplate(editModal.record.id, {
        title_tpl: v.title_tpl,
        body_tpl: v.body_tpl,
        link_tpl: v.link_tpl ?? '',
        channels: (v.channels as string[]).join(','),
        enabled: v.enabled,
      })
      message.success(t('已保存'))
      editModal.close()
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      setSaving(false)
    }
  }

  // 服务端预览/试发只认已保存的模板，所以这两个操作会先把当前表单落库。
  // 有持久化副作用的事实必须让用户可见——按钮上有 Tooltip 说明。
  const handlePreview = async () => {
    if (!editModal.record) return
    const v = await form.validateFields().catch(() => null)
    if (!v) return
    try {
      await updateNotifyTemplate(editModal.record.id, {
        title_tpl: v.title_tpl,
        body_tpl: v.body_tpl,
        link_tpl: v.link_tpl ?? '',
        channels: (v.channels as string[]).join(','),
        enabled: v.enabled,
      })
      const res = await previewNotifyTemplate(editModal.record.id)
      setPreview({
        title: res.title,
        content: res.content,
        link: res.link,
        channels: res.channels || [],
      })
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('预览失败'))
    }
  }

  const handleTestSend = async () => {
    if (!editModal.record) return
    const v = await form.validateFields().catch(() => null)
    if (!v) return
    try {
      await updateNotifyTemplate(editModal.record.id, {
        title_tpl: v.title_tpl,
        body_tpl: v.body_tpl,
        link_tpl: v.link_tpl ?? '',
        channels: (v.channels as string[]).join(','),
        enabled: v.enabled,
      })
      const res = await testSendNotifyTemplate(editModal.record.id)
      message.success(t('已试发到当前账号：{{title}}', { title: res.title }))
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('试发失败'))
    }
  }

  const columns: ColumnsType<NotifyTemplate> = [
    {
      title: t('事件'),
      dataIndex: 'code',
      width: 220,
      responsive: ['sm'],
      render: (code: string) => (
        <div className="notify-template-event">
          <Typography.Text strong className="notify-template-name">{CODE_LABEL[code] ? t(CODE_LABEL[code] ?? '') : code}</Typography.Text>
          <Typography.Text type="secondary" code className="notify-template-code">
            {code}
          </Typography.Text>
        </div>
      ),
    },
    {
      title: t('标题模板'),
      dataIndex: 'title_tpl',
      width: 320,
      ellipsis: true,
      render: (title: string) => (
        <Tooltip title={title}>
          <span className="notify-template-title">{title}</span>
        </Tooltip>
      ),
    },
    {
      title: t('渠道'),
      dataIndex: 'channels',
      width: 160,
      responsive: ['md'],
      render: (chs: string, row) => {
        const list = (chs || row.channel || 'inbox').split(',').filter(Boolean)
        return (
          <Space size={4} wrap className="notify-template-channels">
            {list.map((c) => (
              <Tag key={c} color={c === 'inbox' ? 'blue' : c === 'email' ? 'purple' : c === 'voice' ? 'green' : 'orange'}>
                {c}
              </Tag>
            ))}
          </Space>
        )
      },
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 72,
      align: 'center',
      fixed: 'right',
      render: (en: boolean, row) => (
        <Switch
          checked={en}
          size="small"
          aria-label={t(en ? '停用{{name}}通知模板' : '启用{{name}}通知模板', { name: CODE_LABEL[row.code] || row.code })}
          onChange={async (checked) => {
            try {
              await updateNotifyTemplate(row.id, { enabled: checked })
              message.success(checked ? t('已启用') : t('已停用'))
              void reload()
            } catch {
              message.error(t('更新失败'))
            }
          }}
        />
      ),
    },
    {
      title: t('操作'),
      width: 56,
      align: 'center',
      fixed: 'right',
      render: (_, row) => (
        <Tooltip title={t('编辑模板')}>
          <Button
            type="text"
            size="small"
            className="notify-template-edit-button"
            aria-label={t('编辑通知模板 {{name}}', { name: CODE_LABEL[row.code] || row.code })}
            icon={<EditOutlined />}
            onClick={() => openEdit(row)}
          />
        </Tooltip>
      ),
    },
  ]

  return (
    <ListPageShell
      className="notify-templates-page"
      toolbar={(
        <TableToolbar
          title="通知模板"
          total={list.length}
          icon={<NotificationOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.35)"
          description={t('工单 / IM / CRM 事件站内信文案与渠道；试发到当前账号验证闭环')}
          extra={
            <Space wrap>
              <Tag color="green">{t('站内信 开')}</Tag>
              <Tag color={channels?.email?.enabled ? 'blue' : 'default'}>
                {t('邮件')} {channels?.email?.enabled ? t('开') : t('关')}
              </Tag>
              {/* stub 只写日志、不真发短信，标签必须说清楚，别让人以为通知已送达 */}
              <Tag color={channels?.sms?.enabled ? (channels.sms.provider === 'stub' ? 'default' : 'orange') : 'default'}>
                {t('短信')} {channels?.sms?.enabled ? (channels.sms.provider === 'stub' ? t('演示 stub（不真发）') : t('开')) : t('关')}
              </Tag>
              <Button type="link" href="/system/inbox">
                {t('站内信收件箱')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>
                {t('刷新')}
              </Button>
            </Space>
          }
        />
      )}
      >
      <Tabs
          defaultActiveKey="templates"
          items={[
            {
              key: 'templates',
              label: t('模板'),
              children: (
                <>
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message={t('变量用 {{name}} 占位，如 {{ticket_no}}、{{title}}、{{score}}。邮件/短信需在模板渠道勾选；SMTP 账号在 系统设置 → 通知设置 中配置（DB 热生效，env 兜底）。')}
                  />
                  <Table
                    rowKey="id"
                    className="list-table notify-template-table"
                    loading={loading}
                    columns={columns}
                    dataSource={list}
                    rowClassName={(row) => (row.enabled ? '' : 'notify-template-row-disabled')}
                    scroll={{ x: 'max-content' }}
                    locale={{ emptyText: <GlassEmpty text="暂无模板" /> }}
                    pagination={false}
                  />
                </>
              ),
            },
            ...(isPlatform
              ? [
                  {
                    key: 'deliveries',
                    label: t('发送日志'),
                    children: <DeliveryLogsPanel />,
                  },
                ]
              : []),
          ]}
        />
      <Drawer
        title={editModal.record ? t('编辑 · {{name}}', { name: CODE_LABEL[editModal.record.code] || editModal.record.code }) : t('编辑模板')}
        width={560}
        open={editModal.open}
        onClose={editModal.close}
        destroyOnHidden
        extra={
          <Space>
            <Tooltip title={t('会先保存当前表单内容再渲染预览')}>
              <Button icon={<EyeOutlined />} onClick={() => void handlePreview()}>
                {t('预览')}
              </Button>
            </Tooltip>
            <Tooltip title={t('会先保存当前表单内容再发送')}>
              <Button icon={<SendOutlined />} onClick={() => void handleTestSend()}>
                {t('试发给我')}
              </Button>
            </Tooltip>
            <Button type="primary" loading={saving} onClick={() => void handleSave()}>
              {t('保存')}
            </Button>
          </Space>
        }
      >
        {editModal.record && (
          <Form form={form} layout="vertical">
            <Form.Item label={t('事件码')}>
              <Typography.Text code>{editModal.record.code}</Typography.Text>
            </Form.Item>
            <Form.Item name="title_tpl" label={t('标题模板')} rules={[{ required: true, message: t('必填') }]}>
              <Input maxLength={256} showCount />
            </Form.Item>
            <Form.Item name="body_tpl" label={t('正文模板')} rules={[{ required: true, message: t('必填') }]}>
              <Input.TextArea rows={4} />
            </Form.Item>
            <Form.Item name="link_tpl" label={t('跳转链接模板')}>
              <Input placeholder="/ticket?id={{ticket_id}}" />
            </Form.Item>
            <Form.Item name="channels" label={t('投递渠道')} rules={[{ required: true, message: t('至少选一个') }]}>
              <Checkbox.Group options={CHANNEL_OPTS.map((o) => ({ ...o, label: t(o.label) }))} />
            </Form.Item>
            <Form.Item name="enabled" label={t('启用')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </Form>
        )}

        {preview && (
          <Card size="small" title={t('预览效果')} style={{ marginTop: 16 }}>
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              {preview.title}
            </Typography.Title>
            <Typography.Paragraph>{preview.content}</Typography.Paragraph>
            {preview.link ? (
              <Typography.Text type="secondary">{t('链接')}：{preview.link}</Typography.Text>
            ) : null}
            <div style={{ marginTop: 8 }}>
              <Space size={4}>
                {preview.channels.map((c) => (
                  <Tag key={c}>{c}</Tag>
                ))}
              </Space>
            </div>
          </Card>
        )}
      </Drawer>
    </ListPageShell>
  )
}

// ---------------------------------------------------------------------
// 发送日志（管理视图，仅平台管理员；邮件管理面，路线图第 12 项）
// ---------------------------------------------------------------------

const DELIVERY_STATUS_META: Record<string, { label: string; color: string }> = {
  sent: { label: '已发送', color: 'green' },
  failed: { label: '失败', color: 'red' },
  skipped: { label: '跳过', color: 'default' },
  stub: { label: '演示（未真发）', color: 'orange' },
}

function DeliveryLogsPanel() {
  const { t } = useTranslation()
  const userMap = useUserNameMap()
  const [list, setList] = useState<NotifyDeliveryAdmin[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<{
    channel?: string
    status?: string
    keyword?: string
    page: number
    page_size: number
  }>({ page: 1, page_size: 20 })
  const [keywordDraft, setKeywordDraft] = useState('')

  useEffect(() => {
    let alive = true
    setLoading(true)
    listAllNotifyDeliveries(filter)
      .then((res) => {
        if (!alive) return
        setList(res?.list ?? [])
        setTotal(res?.total ?? 0)
      })
      .catch(() => {})
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [filter])

  const columns: ColumnsType<NotifyDeliveryAdmin> = [
    { title: t('时间'), dataIndex: 'created_at', width: 170, render: formatDateTime, responsive: ['md'] },
    {
      title: t('渠道'),
      dataIndex: 'channel',
      width: 88,
      responsive: ['sm'],
      render: (c: string) => <Tag color={c === 'email' ? 'purple' : 'orange'}>{c}</Tag>,
    },
    {
      title: t('收件人'),
      dataIndex: 'target',
      width: 240,
      render: (t: string | undefined, row) => (
        <div className="notify-delivery-recipient">
          <span>{displayUserName(userMap, row.user_id)}</span>
          {t && (
            <Typography.Text type="secondary" title={t} className="cell-mono">
              {t}
            </Typography.Text>
          )}
        </div>
      ),
    },
    {
      title: t('模板'),
      dataIndex: 'template_code',
      width: 220,
      responsive: ['md'],
      render: (code?: string) =>
        code ? (
          <Space direction="vertical" size={0}>
            <span>{CODE_LABEL[code] ? t(CODE_LABEL[code] ?? '') : code}</span>
            <Typography.Text type="secondary" code style={{ fontSize: 12 }}>
              {code}
            </Typography.Text>
          </Space>
        ) : (
          <Typography.Text type="secondary">—</Typography.Text>
        ),
    },
    { title: t('详情'), dataIndex: 'detail', width: 320, ellipsis: true, responsive: ['lg'] },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 112,
      fixed: 'right',
      render: (s: string) => {
        const meta = DELIVERY_STATUS_META[s] ?? { label: s, color: 'default' }
        return <Tag color={meta.color}>{t(meta.label)}</Tag>
      },
    },
  ]

  return (
    <>
      <Space wrap className="notify-delivery-filters" style={{ marginBottom: 12 }}>
        <Select
          placeholder={t('渠道')}
          allowClear
          style={{ width: 110 }}
          value={filter.channel}
          options={[
            { label: t('邮件'), value: 'email' },
            { label: t('短信'), value: 'sms' },
          ]}
          onChange={(v) => setFilter({ ...filter, page: 1, channel: v })}
        />
        <Select
          placeholder={t('状态')}
          allowClear
          style={{ width: 140 }}
          value={filter.status}
          options={Object.entries(DELIVERY_STATUS_META).map(([value, m]) => ({
            value,
            label: t(m.label),
          }))}
          onChange={(v) => setFilter({ ...filter, page: 1, status: v })}
        />
        <Input
          placeholder={t('收件地址 / 模板码')}
          allowClear
          style={{ width: 200 }}
          value={keywordDraft}
          onChange={(e) => setKeywordDraft(e.target.value)}
          onPressEnter={() => setFilter({ ...filter, page: 1, keyword: keywordDraft.trim() || undefined })}
        />
        <Button
          icon={<SearchOutlined />}
          onClick={() => setFilter({ ...filter, page: 1, keyword: keywordDraft.trim() || undefined })}
        >
          {t('查询')}
        </Button>
        <Button icon={<ReloadOutlined />} onClick={() => setFilter({ ...filter })}>
          {t('刷新')}
        </Button>
      </Space>
      <Table
        rowKey="id"
        size="small"
        className="list-table notify-delivery-table"
        loading={loading}
        columns={columns}
        dataSource={list}
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: <GlassEmpty text="暂无发送记录" compact /> }}
        pagination={{
          total,
          current: filter.page,
          pageSize: filter.page_size,
          showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (page, page_size) => setFilter({ ...filter, page, page_size }),
        }}
      />
    </>
  )
}
