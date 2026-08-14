import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Descriptions, Form, Grid, Input, Modal, Select, Space, Table, Tag, Typography } from 'antd'
import { ApiOutlined, DeleteOutlined, EditOutlined, KeyOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as WebhookAPI from '@/api/system/webhook'
import type { WebhookDelivery, WebhookMutation, WebhookSubscription } from '@/api/system/webhook'
import EntityDetailDrawer from '@/components/EntityDetailDrawer'
import EntityFormModal from '@/components/EntityFormModal'
import GlassEmpty from '@/components/GlassEmpty'
import ListPageShell from '@/components/ListPageShell'
import StatusPill from '@/components/StatusPill'
import TableRowActions from '@/components/TableRowActions'
import TableToolbar from '@/components/TableToolbar'
import { useCrudModal } from '@/hooks/useCrudModal'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { message } from '@/utils/feedback'
import { formatDateTime } from '@/utils/format'
import './styles.css'

const ACTION_OPTIONS = [
  { label: '全部审计动作', value: '*' },
  { label: '创建', value: 'create' },
  { label: '更新', value: 'update' },
  { label: '删除', value: 'delete' },
  { label: '导出', value: 'export' },
]

const deliveryColors: Record<string, string> = { sent: 'green', failed: 'red', retrying: 'orange', pending: 'blue' }

export default function WebhooksPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [params, setParams] = useState({ page: 1, page_size: 10 })
  const modal = useCrudModal<WebhookSubscription>()
  const [form] = Form.useForm<WebhookMutation>()
  const [submitting, setSubmitting] = useState(false)
  const [secret, setSecret] = useState('')
  const [deliverySubscription, setDeliverySubscription] = useState<WebhookSubscription | null>(null)

  const fetchList = useCallback((p: typeof params) => WebhookAPI.listWebhooks(p), [])
  const onLoadError = useCallback(() => message.error(t('加载 Webhook 订阅失败')), [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ status: 1, event_actions: ['*'] })
    modal.openCreate()
  }
  const openEdit = (record: WebhookSubscription) => {
    form.setFieldsValue({ name: record.name, endpoint_url: record.endpoint_url, event_actions: record.event_actions, status: record.status })
    modal.openEdit(record)
  }
  const submit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (modal.record) {
        await WebhookAPI.updateWebhook(modal.record.id, values)
        message.success(t('Webhook 订阅已更新'))
      } else {
        const result = await WebhookAPI.createWebhook(values)
        setSecret(result.secret)
        message.success(t('Webhook 订阅已创建'))
      }
      modal.close()
      void reload()
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('保存失败'))
    } finally {
      setSubmitting(false)
    }
  }
  const remove = async (id: number) => {
    try { await WebhookAPI.deleteWebhook(id); message.success(t('Webhook 订阅已删除')); void reload() }
    catch { message.error(t('删除失败')) }
  }
  const resetSecret = async (record: WebhookSubscription) => {
    try { const result = await WebhookAPI.resetWebhookSecret(record.id); setSecret(result.secret); message.success(t('签名密钥已重置')) }
    catch { message.error(t('重置密钥失败')) }
  }

  const columns: ColumnsType<WebhookSubscription> = [
    { title: t('名称'), dataIndex: 'name', width: 180, render: (v) => <span className="list-primary-cell">{v}</span> },
    { title: t('端点'), dataIndex: 'endpoint_url', width: 360, ellipsis: true, render: (v) => <Typography.Text copyable={{ text: v }} className="cell-mono webhook-endpoint">{v}</Typography.Text> },
    { title: t('事件'), dataIndex: 'event_actions', width: 220, responsive: ['md'], render: (actions: string[]) => <Space size={4} wrap>{actions.map((action) => <Tag key={action}>{action === '*' ? t('全部') : action}</Tag>)}</Space> },
    { title: t('状态'), dataIndex: 'status', width: 90, render: (value) => value === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" /> },
    { title: t('连续失败'), dataIndex: 'consecutive_failures', width: 100, responsive: ['lg'], render: (v) => v || 0 },
    { title: t('最近投递'), dataIndex: 'last_delivered_at', width: 170, responsive: ['lg'], render: formatDateTime },
    {
      title: t('操作'),
      width: compactActions ? 48 : 150,
      fixed: 'right',
      align: 'center',
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: row.name })}
          className="webhook-row-actions"
          actions={[
            {
              key: 'deliveries',
              label: t('查看投递'),
              icon: <ApiOutlined />,
              onClick: () => setDeliverySubscription(row),
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:webhook:update'),
              onClick: () => openEdit(row),
            },
            {
              key: 'reset-secret',
              label: t('重置密钥'),
              icon: <KeyOutlined />,
              show: hasPerm('system:webhook:reset-secret'),
              confirm: t('旧密钥将立即失效，确认重置？'),
              onClick: () => { void resetSecret(row) },
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:webhook:delete'),
              confirm: t('确认删除该 Webhook 订阅？'),
              onClick: () => { void remove(row.id) },
            },
          ]}
        />
      ),
    },
  ]

  return <>
    <ListPageShell
      className="webhooks-page"
      toolbar={<TableToolbar title="Webhook 订阅" total={total} icon={<ApiOutlined />} description={t('将审计事件可靠推送到外部 HTTPS 端点，支持 HMAC 签名、重试与自动停投')} extra={<Space><Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>{hasPerm('system:webhook:create') && <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建订阅')}</Button>}</Space>} />}
    >
      <Alert type="info" showIcon message={t('签名密钥仅在创建或重置后显示一次；请求携带 X-GAK-Event-ID、X-GAK-Timestamp 与 X-GAK-Signature。')} />
      <Table rowKey="id" className="list-table webhook-table" loading={loading} columns={columns} dataSource={list} scroll={{ x: 'max-content' }} locale={{ emptyText: <GlassEmpty text="暂无 Webhook 订阅" compact /> }} pagination={{ total, current: params.page, pageSize: params.page_size, showSizeChanger: true, showTotal: (n) => t('共 {{n}} 条', { n }), onChange: (page, page_size) => setParams({ page, page_size }) }} />
    </ListPageShell>

    <EntityFormModal title={modal.record ? t('编辑 Webhook 订阅') : t('新建 Webhook 订阅')} open={modal.open} form={form} onClose={modal.close} onSubmit={submit} submitting={submitting} width={640}>
      <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('必填') }]}><Input maxLength={128} /></Form.Item>
      <Form.Item name="endpoint_url" label={t('HTTPS 端点')} rules={[{ required: true, message: t('必填') }, { type: 'url', message: t('请输入有效 URL') }]} extra={t('生产环境仅允许公网 HTTPS；不跟随重定向。')}><Input placeholder="https://example.com/webhooks/audit" maxLength={2048} /></Form.Item>
      <Form.Item name="event_actions" label={t('审计动作')} rules={[{ required: true, message: t('至少选择一项') }]}><Select mode="tags" options={ACTION_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} tokenSeparators={[',']} maxCount={50} /></Form.Item>
      <Form.Item name="status" label={t('状态')}><Select options={[{ label: t('启用'), value: 1 }, { label: t('停用'), value: 0 }]} /></Form.Item>
    </EntityFormModal>

    <Modal title={t('请立即保存签名密钥')} open={Boolean(secret)} footer={<Button type="primary" onClick={() => setSecret('')}>{t('我已安全保存')}</Button>} closable={false} maskClosable={false}>
      <Alert type="warning" showIcon message={t('关闭后无法再次查看，只能重置密钥。')} />
      <Typography.Paragraph copyable={{ text: secret }} className="webhook-secret"><code>{secret}</code></Typography.Paragraph>
    </Modal>

    <EntityDetailDrawer title={t('投递记录 · {{name}}', { name: deliverySubscription?.name || '' })} open={Boolean(deliverySubscription)} onClose={() => setDeliverySubscription(null)} width="min(900px, 100vw)">
      {deliverySubscription && <DeliveryPanel subscription={deliverySubscription} />}
    </EntityDetailDrawer>
  </>
}

function DeliveryPanel({ subscription }: { subscription: WebhookSubscription }) {
  const { t } = useTranslation()
  const [params, setParams] = useState({ subscription_id: subscription.id, page: 1, page_size: 10 })
  const fetcher = useCallback((p: typeof params) => WebhookAPI.listWebhookDeliveries(p), [])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher })
  const columns: ColumnsType<WebhookDelivery> = [
    { title: t('事件 ID'), dataIndex: 'event_id', width: 190, render: (v) => <Typography.Text copyable={{ text: v }} className="cell-mono">{v}</Typography.Text> },
    { title: t('动作'), dataIndex: 'event_action', width: 120 },
    { title: t('状态'), dataIndex: 'status', width: 100, render: (v) => <Tag color={deliveryColors[v] || 'default'}>{v}</Tag> },
    { title: t('尝试次数'), dataIndex: 'attempts', width: 90 },
    { title: 'HTTP', dataIndex: 'response_status', width: 80, render: (v) => v || '—' },
    { title: t('错误'), dataIndex: 'last_error', width: 280, ellipsis: true },
    { title: t('时间'), dataIndex: 'created_at', width: 170, render: formatDateTime },
  ]
  return <><Descriptions size="small" bordered column={1} items={[{ key: 'endpoint', label: t('端点'), children: subscription.endpoint_url }, { key: 'error', label: t('最近错误'), children: subscription.last_error || '—' }]} /><Table rowKey="id" size="small" className="list-table webhook-delivery-table" loading={loading} columns={columns} dataSource={list} scroll={{ x: 'max-content' }} title={() => <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>} pagination={{ total, current: params.page, pageSize: params.page_size, onChange: (page, page_size) => setParams({ ...params, page, page_size }) }} /></>
}
