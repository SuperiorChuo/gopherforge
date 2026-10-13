// 支付中心管理页：渠道配置 / 订单列表 / 退款管理 / 回调任务，四 Tab 单页。
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert,
  Button,
  Card,
  Col,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography, Grid } from 'antd'
import { PlusOutlined, ReloadOutlined, StopOutlined, TransactionOutlined, EditOutlined, DeleteOutlined} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  formatCents,
  payApi,
  type PayCallbackJob,
  type PayChannel,
  type PayNotifyLog,
  type PayOrder,
  type PayRefund,
  type PayStats,
} from '@/api/pay'
import GlassEmpty from '@/components/common/GlassEmpty'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

const ORDER_STATUS_META: Record<string, { label: string; color: string }> = {
  waiting: { label: '待支付', color: 'gold' },
  paying: { label: '支付中', color: 'blue' },
  paid: { label: '已支付', color: 'green' },
  closed: { label: '已关闭', color: 'default' },
}

const REFUND_STATUS_META: Record<string, { label: string; color: string }> = {
  pending: { label: '待处理', color: 'gold' },
  processing: { label: '退款中', color: 'blue' },
  refunded: { label: '已退款', color: 'green' },
  failed: { label: '失败', color: 'red' },
}

const CHANNEL_NAME: Record<string, string> = {
  mock: 'Mock 模拟渠道',
  wechat_native: '微信 Native 扫码',
  wechat_jsapi: '微信 JSAPI',
  alipay_pc: '支付宝 PC',
  alipay_wap: '支付宝 WAP',
}

export default function PayCenterPage() {
  const { t } = useTranslation()
  const [stats, setStats] = useState<PayStats | null>(null)

  const loadStats = useCallback(async () => {
    try {
      setStats(await payApi.stats())
    } catch {
      /* 顶卡失败不阻断页面 */
    }
  }, [])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

  return (
    <div>
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={4}>
          <Card><Statistic title={t('待支付')} value={stats?.waiting ?? 0} /></Card>
        </Col>
        <Col span={4}>
          <Card><Statistic title={t('支付中')} value={stats?.paying ?? 0} /></Card>
        </Col>
        <Col span={4}>
          <Card><Statistic title={t('已支付')} value={stats?.paid ?? 0} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title={t('累计实收')} value={formatCents(stats?.paid_amount_cents ?? 0)} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title={t('累计退款')} value={formatCents(stats?.refunded_amount_cents ?? 0)} /></Card>
        </Col>
      </Row>
      <Card>
        <Tabs
          defaultActiveKey="orders"
          onChange={() => void loadStats()}
          items={[
            { key: 'orders', label: t('支付订单'), children: <OrdersTab onChanged={loadStats} /> },
            { key: 'refunds', label: t('退款管理'), children: <RefundsTab onChanged={loadStats} /> },
            { key: 'channels', label: t('渠道配置'), children: <ChannelsTab /> },
            { key: 'callbacks', label: t('回调任务'), children: <CallbacksTab /> },
          ]}
        />
      </Card>
    </div>
  )
}

function OrdersTab({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [list, setList] = useState<PayOrder[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string>()
  const [loading, setLoading] = useState(false)
  const [refundOrder, setRefundOrder] = useState<PayOrder | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await payApi.listOrders({ status, page, page_size: 20 })
      setList(res.list)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }, [status, page])

  useEffect(() => {
    void load()
  }, [load])

  const close = async (orderNo: string) => {
    await payApi.closeOrder(orderNo)
    message.success(t('已关单'))
    void load()
    onChanged()
  }

  return (
    <>
      <TableToolbar title="支付订单" total={total} />
      <div style={{ marginBottom: 12, display: 'flex', gap: 8 }}>
        <Select
          allowClear
          placeholder={t('状态筛选')}
          style={{ width: 140 }}
          value={status}
          onChange={(v) => { setStatus(v); setPage(1) }}
          options={Object.entries(ORDER_STATUS_META).map(([value, m]) => ({ value, label: t(m.label) }))}
        />
        <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
      </div>
      <Table<PayOrder>
        rowKey="id"
        size="middle"
        loading={loading}
        dataSource={list}
        locale={{ emptyText: <GlassEmpty text="暂无支付订单" /> }}
        pagination={{ current: page, pageSize: 20, total, onChange: setPage, showTotal: (n) => t('共 {{n}} 单', { n }) }}
        columns={[
          { title: t('订单号'), dataIndex: 'order_no', width: 210, render: (v: string) => <Text copyable={{ text: v }}>{v}</Text> },
          { title: t('业务'), width: 150, render: (_, r) => <Text type="secondary">{r.biz_type}/{r.biz_id}</Text> },
          { title: t('标题'), dataIndex: 'subject', ellipsis: true },
          { title: t('金额'), dataIndex: 'amount_cents', width: 110, render: (v: number) => formatCents(v) },
          {
            title: t('已退'), dataIndex: 'refunded_cents', width: 110,
            render: (v: number) => (v > 0 ? <Text type="warning">{formatCents(v)}</Text> : <Text type="secondary">—</Text>),
          },
          {
            title: t('状态'), dataIndex: 'status', width: 100,
            render: (v: string) => { const m = ORDER_STATUS_META[v]; return m ? <Tag color={m.color}>{t(m.label)}</Tag> : <Tag>{v}</Tag> },
          },
          { title: t('渠道'), dataIndex: 'channel_code', width: 130, render: (v?: string) => (v ? (CHANNEL_NAME[v] ? t(CHANNEL_NAME[v]) : v) : <Text type="secondary">—</Text>) },
          { title: t('创建时间'), dataIndex: 'created_at', width: 170, render: (v: string) => formatDateTime(v) },
          {
            title: t('操作'), width: compactActions ? 48 : 88, fixed: 'right' as const, align: 'center' as const,
            render: (_, r) => (
              <TableRowActions
                menuOnly={compactActions}
                maxInline={2}
                actions={[
                  {
                    key: 'close',
                    label: t('关单'),
                    icon: <StopOutlined />,
                    danger: true,
                    show: r.status === 'waiting' || r.status === 'paying',
                    confirm: t('确认关闭该订单？'),
                    onClick: () => { void close(r.order_no) },
                  },
                  {
                    key: 'refund',
                    label: t('退款'),
                    icon: <TransactionOutlined />,
                    show: r.status === 'paid' && r.refunded_cents < r.amount_cents,
                    onClick: () => setRefundOrder(r),
                  },
                ]}
              />
            ),
          },
        ]}
      />
      <RefundModal
        order={refundOrder}
        onClose={() => setRefundOrder(null)}
        onDone={() => { setRefundOrder(null); void load(); onChanged() }}
      />
    </>
  )
}

function RefundModal({ order, onClose, onDone }: { order: PayOrder | null; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation()
    const [form] = Form.useForm<{ amount: number; reason?: string }>()
  const remain = order ? order.amount_cents - order.refunded_cents : 0

  useEffect(() => {
    if (order) form.setFieldsValue({ amount: remain / 100, reason: undefined })
  }, [order, remain, form])

  const submit = async () => {
    if (!order) return
    const v = await form.validateFields()
    await payApi.createRefund({
      order_no: order.order_no,
      amount_cents: Math.round(v.amount * 100),
      reason: v.reason,
    })
    message.success(t('退款已发起'))
    onDone()
  }

  return (
    <Modal open={!!order} title={t('退款 · {{no}}', { no: order?.order_no ?? '' })} onCancel={onClose} onOk={() => void submit()} destroyOnHidden>
      <Form form={form} layout="vertical">
        <Form.Item label={t('退款金额（可退 {{amount}}）', { amount: formatCents(remain) })} name="amount" rules={[{ required: true, message: t('请输入金额') }]}>
          <InputNumber min={0.01} max={remain / 100} precision={2} prefix="¥" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label={t('退款原因')} name="reason">
          <Input.TextArea rows={2} maxLength={200} placeholder={t('选填')} />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function RefundsTab({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation()
    const [list, setList] = useState<PayRefund[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await payApi.listRefunds({ page, page_size: 20 })
      setList(res.list)
      setTotal(res.total)
      onChanged()
    } finally {
      setLoading(false)
    }
  }, [page, onChanged])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <Table<PayRefund>
      rowKey="id"
      size="middle"
      loading={loading}
      dataSource={list}
      locale={{ emptyText: <GlassEmpty text="暂无退款单" /> }}
      pagination={{ current: page, pageSize: 20, total, onChange: setPage }}
      columns={[
        { title: t('退款单号'), dataIndex: 'refund_no', width: 210 },
        { title: t('订单号'), dataIndex: 'order_no', width: 210 },
        { title: t('金额'), dataIndex: 'amount_cents', width: 110, render: (v: number) => formatCents(v) },
        {
          title: t('状态'), dataIndex: 'status', width: 100,
          render: (v: string) => { const m = REFUND_STATUS_META[v]; return m ? <Tag color={m.color}>{t(m.label)}</Tag> : <Tag>{v}</Tag> },
        },
        { title: t('原因'), dataIndex: 'reason', ellipsis: true, render: (v?: string) => v || <Text type="secondary">—</Text> },
        { title: t('完成时间'), dataIndex: 'finished_at', width: 170, render: (v?: string) => (v ? formatDateTime(v) : '—') },
      ]}
    />
  )
}

function ChannelsTab() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [list, setList] = useState<PayChannel[]>([])
  const [codes, setCodes] = useState<string[]>([])
  const [editing, setEditing] = useState<PayChannel | 'new' | null>(null)
  const [form] = Form.useForm<Partial<PayChannel>>()

  const load = useCallback(async () => {
    setList(await payApi.listChannels())
  }, [])

  useEffect(() => {
    void load()
    // 渠道类型只认后端注册表（生产不注册 mock）。取不到就是空，不能兜底成
    // ['mock']——那会让管理员以为本部署支持一个实际不存在的渠道。
    payApi.channelCodes().then((v) => setCodes(v ?? [])).catch(() => setCodes([]))
  }, [load])

  const openEdit = (ch: PayChannel | 'new') => {
    setEditing(ch)
    if (ch === 'new') form.resetFields()
    else form.setFieldsValue(ch)
  }

  const submit = async () => {
    const v = await form.validateFields()
    if (editing === 'new') {
      await payApi.createChannel(v)
      message.success(t('渠道已创建'))
    } else if (editing) {
      await payApi.updateChannel(editing.id, v)
      message.success(t('渠道已更新'))
    }
    setEditing(null)
    void load()
  }

  // 后端一个渠道适配器都没注册（生产默认不注册 mock，微信/支付宝尚未实现）：
  // 如实告知，而不是让管理员对着空下拉框反复试。
  const noAdapters = codes.length === 0

  return (
    <>
      {noAdapters ? (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message={t('本部署没有可用的支付渠道适配器')}
          description={t('收银台将无渠道可选、无法收款。演示/联调环境可给 pay-service 设 PAY_DEMO_MODE=true 启用 mock 演示渠道；生产环境需要先接入真实渠道适配器。')}
        />
      ) : null}
      <Button
        type="primary"
        icon={<PlusOutlined />}
        style={{ marginBottom: 12 }}
        disabled={noAdapters}
        onClick={() => openEdit('new')}
      >
        {t('新增渠道')}
      </Button>
      <Table<PayChannel>
        rowKey="id"
        size="middle"
        dataSource={list}
        locale={{ emptyText: <GlassEmpty text={noAdapters ? '本部署未启用任何支付渠道适配器' : '暂无渠道，点「新增渠道」添加'} /> }}
        pagination={false}
        columns={[
          { title: t('类型'), dataIndex: 'code', width: 160, render: (v: string) => (CHANNEL_NAME[v] ? t(CHANNEL_NAME[v]) : v) },
          { title: t('名称'), dataIndex: 'name' },
          {
            title: t('状态'), dataIndex: 'status', width: 90,
            render: (v: number) => (v === 1 ? <Tag color="green">{t('启用')}</Tag> : <Tag>{t('停用')}</Tag>),
          },
          { title: t('费率'), dataIndex: 'fee_rate_bps', width: 100, render: (v: number) => `${(v / 100).toFixed(2)}%` },
          { title: t('备注'), dataIndex: 'remark', ellipsis: true, render: (v?: string) => v || '—' },
          {
            title: t('操作'), width: compactActions ? 48 : 88, align: 'center' as const, fixed: 'right' as const,
            render: (_, r) => (
              <TableRowActions
                menuOnly={compactActions}
                maxInline={2}
                actions={[
                  { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(r) },
                  {
                    key: 'delete',
                    label: t('删除'),
                    icon: <DeleteOutlined />,
                    danger: true,
                    confirm: t('确认删除该渠道？'),
                    onClick: () => { void payApi.deleteChannel(r.id).then(() => void load()) },
                  },
                ]}
              />
            ),
          },
        ]}
      />
      <Drawer title={editing === 'new' ? t('新增渠道') : t('编辑渠道')} open={!!editing} onClose={() => setEditing(null)} width={420}
        extra={<Button type="primary" onClick={() => void submit()}>{t('保存')}</Button>}>
        <Form form={form} layout="vertical">
          <Form.Item label={t('渠道类型')} name="code" rules={[{ required: true, message: t('请选择类型') }]}>
            <Select
              disabled={editing !== 'new'}
              options={codes.map((c) => ({ value: c, label: CHANNEL_NAME[c] ? t(CHANNEL_NAME[c]) : c }))}
            />
          </Form.Item>
          <Form.Item label={t('显示名称')} name="name" rules={[{ required: true, message: t('请输入名称') }]}>
            <Input maxLength={50} placeholder={t('收银台展示给用户的名称')} />
          </Form.Item>
          <Form.Item label={t('状态')} name="status" initialValue={1}>
            <Select options={[{ value: 1, label: t('启用') }, { value: 2, label: t('停用') }]} />
          </Form.Item>
          <Form.Item label={t('费率（万分位）')} name="fee_rate_bps" initialValue={0} tooltip={t('仅展示与对账参考，60 = 0.60%')}>
            <InputNumber min={0} max={10000} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label={t('备注')} name="remark">
            <Input.TextArea rows={2} maxLength={200} />
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}

function CallbacksTab() {
  const { t } = useTranslation()
    const [jobs, setJobs] = useState<PayCallbackJob[]>([])
  const [logs, setLogs] = useState<PayNotifyLog[]>([])

  const load = useCallback(async () => {
    setJobs(await payApi.listCallbackJobs())
    setLogs(await payApi.listNotifyLogs())
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <>
      <Typography.Title level={5}>{t('业务回调任务')}</Typography.Title>
      <Table<PayCallbackJob>
        rowKey="id"
        size="small"
        dataSource={jobs}
        locale={{ emptyText: <GlassEmpty text="无待处理回调（成功即删，仅失败重试/死信可见）" /> }}
        pagination={false}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          { title: t('类型'), dataIndex: 'kind', width: 90 },
          { title: t('订单ID'), dataIndex: 'order_id', width: 100 },
          { title: t('已试'), dataIndex: 'attempts', width: 70 },
          {
            title: t('状态'), dataIndex: 'status', width: 90,
            render: (v: string) => (v === 'dead' ? <Tag color="red">{t('死信')}</Tag> : <Tag color="blue">{t('待投递')}</Tag>),
          },
          { title: t('最近错误'), dataIndex: 'last_error', ellipsis: true, render: (v?: string) => v || '—' },
          {
            title: t('操作'), width: 48, align: 'center' as const,
            render: (_, r) =>
              r.status === 'dead' ? (
                <TableRowActions
                  maxInline={1}
                  actions={[
                    {
                      key: 'replay',
                      label: t('重推'),
                      icon: <ReloadOutlined />,
                      onClick: () => {
                        void payApi.replayCallbackJob(r.id).then(() => {
                          message.success(t('已重推'))
                          void load()
                        })
                      },
                    },
                  ]}
                />
              ) : null,
          },
        ]}
      />
      <Typography.Title level={5} style={{ marginTop: 24 }}>{t('渠道回调日志')}</Typography.Title>
      <Table<PayNotifyLog>
        rowKey="id"
        size="small"
        dataSource={logs}
        locale={{ emptyText: <GlassEmpty text="暂无渠道回调" /> }}
        pagination={false}
        columns={[
          { title: t('时间'), dataIndex: 'created_at', width: 170, render: (v: string) => formatDateTime(v) },
          { title: t('渠道'), dataIndex: 'channel_code', width: 110 },
          { title: t('订单号'), dataIndex: 'order_no', width: 210 },
          {
            title: t('验签'), dataIndex: 'sign_ok', width: 80,
            render: (v: boolean) => (v ? <Tag color="green">{t('通过')}</Tag> : <Tag color="red">{t('失败')}</Tag>),
          },
          {
            title: t('结论'), dataIndex: 'result', width: 140,
            render: (v: string) => (v === 'applied' ? <Tag color="green">{t('已入账')}</Tag> : v === 'duplicate' ? <Tag>{t('重复')}</Tag> : <Tag color="orange">{v}</Tag>),
          },
        ]}
      />
    </>
  )
}
