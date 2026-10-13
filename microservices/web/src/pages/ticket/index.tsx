import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Tag,
  Typography,
} from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { message } from '@/utils/feedback'
import {
  createTicket,
  getTicketSLAPolicy,
  getTicketStats,
  listTickets,
  PRIORITY_META,
  SOURCE_META,
  STATUS_META,
  type Ticket,
  type TicketSLAPolicy,
  type TicketStats,
} from '@/api/ticket'
import TicketDetailDrawer, { isTicketOverdue } from '@/components/ticket/TicketDetailDrawer'
import { notifyTicketStatsRefresh } from '@/components/ticket/TicketMineBadge'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

interface SearchParams {
  status?: string
  priority?: string
  source?: string
  scope?: string
  keyword?: string
  overdue?: string
  id?: string
  page: number
  page_size: number
}

const STATUS_GRADIENT: Record<string, string> = {
  open: 'linear-gradient(90deg, #1677ff, #69b1ff)',
  in_progress: 'linear-gradient(90deg, #fa8c16, #ffc53d)',
  resolved: 'linear-gradient(90deg, #52c41a, #95de64)',
  closed: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
}

export default function TicketPage() {
  const { t } = useTranslation()
  const statusOptions = useMemo(
    () => Object.entries(STATUS_META).map(([value, m]) => ({ value, label: t(m.label) })),
    [t],
  )
  const priorityOptions = useMemo(
    () => Object.entries(PRIORITY_META).map(([value, m]) => ({ value, label: t(m.label) })),
    [t],
  )
  const sourceOptions = useMemo(
    () => Object.entries(SOURCE_META).map(([value, label]) => ({ value, label: t(label) })),
    [t],
  )
  const scopeOptions = useMemo(
    () => [
      { value: 'all', label: t('全部') },
      { value: 'mine', label: t('我的') },
      { value: 'unassigned', label: t('未指派') },
    ],
    [t],
  )
  const formatSLAHours = (h: number) => {
    if (h <= 0) return t('不自动设截止')
    if (h < 24) return t('{{n}} 小时', { n: h })
    if (h % 24 === 0) return t('{{n}} 天', { n: h / 24 })
    return t('{{n}} 小时', { n: h })
  }
  const [list, setList] = useState<Ticket[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [stats, setStats] = useState<TicketStats | null>(null)
  const [sla, setSla] = useState<TicketSLAPolicy | null>(null)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 12 })

  const detailId = params.id ? Number(params.id) : null
  const detailOpen = !!(detailId && detailId > 0)

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listTickets({
        status: p.status,
        priority: p.priority,
        source: p.source,
        keyword: p.keyword,
        overdue: p.overdue === '1' ? '1' : undefined,
        scope: p.scope === 'all' ? undefined : p.scope,
        page: p.page,
        page_size: p.page_size,
      })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取工单失败'))
    } finally {
      setLoading(false)
    }
  }

  const fetchStats = async () => {
    try {
      setStats(await getTicketStats())
      notifyTicketStatsRefresh()
    } catch {
      /* ignore */
    }
  }

  // 逐字段列举比整个 params 更精确（fetchList 身份不稳定）；刻意细粒度依赖。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    void fetchList(params)
  }, [params.page, params.page_size, params.status, params.priority, params.source, params.scope, params.keyword, params.overdue])

  // 挂载时初始化一次统计/SLA/表单回填；params 初始值即所求，刻意不进依赖。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    void fetchStats()
    getTicketSLAPolicy()
      .then(setSla)
      .catch(() => {})
    searchForm.setFieldsValue({
      status: params.status,
      priority: params.priority,
      source: params.source,
      scope: params.scope || 'all',
      keyword: params.keyword,
      overdue: params.overdue === '1' ? '1' : undefined,
    })
  }, [])

  function openDetail(row: Ticket) {
    setParams({ ...params, id: String(row.id) })
  }

  function closeDetail() {
    const next = { ...params }
    delete next.id
    setParams(next)
  }

  async function onCreate() {
    const values = await createForm.validateFields()
    try {
      const due =
        values.due_at && dayjs.isDayjs(values.due_at)
          ? values.due_at.endOf('day').toISOString()
          : undefined
      const tk = await createTicket({
        title: values.title,
        description: values.description,
        priority: values.priority || 'normal',
        source: values.source || 'manual',
        contact_name: values.contact_name,
        contact_phone: values.contact_phone,
        customer_id: values.customer_id,
        due_at: due,
      })
      message.success(
        tk.due_at && !due
          ? t('已创建 {{n}}（已按优先级套用 SLA 截止）', { n: tk.ticket_no })
          : t('已创建 {{n}}', { n: tk.ticket_no }),
      )
      setCreateOpen(false)
      createForm.resetFields()
      void fetchList(params)
      void fetchStats()
      openDetail(tk)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建失败'))
    }
  }

  const onSearch = async () => {
    const v = await searchForm.validateFields().catch(() => searchForm.getFieldsValue())
    setParams({
      ...params,
      page: 1,
      status: v.status || undefined,
      priority: v.priority || undefined,
      source: v.source || undefined,
      scope: v.scope && v.scope !== 'all' ? v.scope : undefined,
      keyword: v.keyword || undefined,
      overdue: v.overdue === '1' || v.overdue === true ? '1' : undefined,
    })
  }

  const cards = useMemo(() => list, [list])

  const slaHint = sla
    ? t('默认 SLA：紧急 {{a}} · 高 {{b}} · 普通 {{c}} · 低 {{d}}（未选手动截止时自动填入）', {
        a: formatSLAHours(sla.urgent_hours),
        b: formatSLAHours(sla.high_hours),
        c: formatSLAHours(sla.normal_hours),
        d: formatSLAHours(sla.low_hours),
      })
    : null

  return (
    <div className="page-wrap">
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card" hoverable onClick={() => setParams({ ...params, page: 1, status: 'open', overdue: undefined })}>
            <Statistic title={t('待处理')} value={stats?.open ?? 0} valueStyle={{ color: 'var(--c-primary)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card" hoverable onClick={() => setParams({ ...params, page: 1, status: 'in_progress', overdue: undefined })}>
            <Statistic title={t('处理中')} value={stats?.in_progress ?? 0} valueStyle={{ color: 'var(--c-orange)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card" hoverable onClick={() => setParams({ ...params, page: 1, status: undefined, overdue: '1' })}>
            <Statistic title={t('已超时')} value={stats?.overdue ?? 0} valueStyle={{ color: 'var(--c-error)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card" hoverable onClick={() => setParams({ ...params, page: 1, scope: 'mine', status: undefined, overdue: undefined })}>
            <Statistic title={t('我的未结')} value={stats?.mine_open ?? 0} valueStyle={{ color: 'var(--c-purple)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card
            size="small"
            className="glass-card"
            hoverable
            onClick={() => setParams({ ...params, page: 1, scope: 'mine', overdue: '1', status: undefined })}
          >
            <Statistic title={t('我的超时')} value={stats?.mine_overdue ?? 0} valueStyle={{ color: 'var(--c-error-strong)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('已解决')} value={stats?.resolved ?? 0} valueStyle={{ color: 'var(--c-success)' }} />
          </Card>
        </Col>
      </Row>

      <Card
        className="glass-card"
        title={t('工单中心')}
        extra={
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => {
                void fetchList(params)
                void fetchStats()
              }}
            >
              {t('刷新')}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              {t('新建工单')}
            </Button>
          </Space>
        }
      >
        {slaHint ? (
          <Text type="secondary" style={{ display: 'block', marginBottom: 12, fontSize: 12 }}>
            {slaHint}
          </Text>
        ) : null}

        <Form form={searchForm} layout="inline" style={{ marginBottom: 16, rowGap: 8 }} onFinish={() => void onSearch()}>
          <Form.Item name="keyword">
            <Input allowClear placeholder={t('单号/标题/联系人')} prefix={<SearchOutlined />} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select allowClear placeholder={t('状态')} options={statusOptions} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="priority">
            <Select allowClear placeholder={t('优先级')} options={priorityOptions} style={{ width: 110 }} />
          </Form.Item>
          <Form.Item name="source">
            <Select allowClear placeholder={t('来源')} options={sourceOptions} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="scope" initialValue="all">
            <Select options={scopeOptions} style={{ width: 110 }} />
          </Form.Item>
          <Form.Item name="overdue">
            <Select
              allowClear
              placeholder={t('超时筛选')}
              style={{ width: 110 }}
              options={[{ value: '1', label: t('仅超时') }]}
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              {t('查询')}
            </Button>
          </Form.Item>
        </Form>

        {loading && cards.length === 0 ? (
          <GlassEmpty text="加载中…" />
        ) : cards.length === 0 ? (
          <GlassEmpty text="暂无工单，可从 IM 坐席台一键转入或点右上角新建" />
        ) : (
          <Row gutter={[16, 16]}>
            {cards.map((card) => {
              const sm = STATUS_META[card.status] || { label: card.status, color: 'default' }
              const pm = PRIORITY_META[card.priority] || { label: card.priority, color: 'default' }
              const overdue = isTicketOverdue(card)
              return (
                <Col key={card.id} xs={24} sm={12} lg={8} xl={6}>
                  <Card
                    hoverable
                    className={`glass-card${overdue ? ' ticket-card-overdue' : ''}`}
                    onClick={() => openDetail(card)}
                    styles={{ body: { paddingTop: 12 } }}
                  >
                    <div
                      style={{
                        height: 4,
                        borderRadius: 2,
                        margin: '-12px -12px 12px',
                        background: overdue
                          ? 'linear-gradient(90deg, #ff4d4f, #ff7875)'
                          : STATUS_GRADIENT[card.status] || STATUS_GRADIENT.open,
                      }}
                    />
                    <Space direction="vertical" size={6} style={{ width: '100%' }}>
                      <Space wrap>
                        <Text code>{card.ticket_no}</Text>
                        <Tag color={sm.color}>{t(sm.label)}</Tag>
                        <Tag color={pm.color}>{t(pm.label)}</Tag>
                        {overdue ? <Tag color="red">{t('超时')}</Tag> : null}
                      </Space>
                      <Text strong ellipsis>
                        {card.title}
                      </Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {t(SOURCE_META[card.source] ?? '') || card.source}
                        {card.contact_name ? ` · ${card.contact_name}` : ''}
                        {card.contact_phone ? ` · ${card.contact_phone}` : ''}
                      </Text>
                      <Text type="secondary" style={{ fontSize: 12, color: overdue ? 'var(--c-error)' : undefined }}>
                        {formatDateTime(card.created_at)}
                        {card.due_at ? ` · ${t('截止')} ${formatDateTime(card.due_at)}` : ''}
                      </Text>
                    </Space>
                  </Card>
                </Col>
              )
            })}
          </Row>
        )}

        <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end' }}>
          <Space>
            <Text type="secondary">{t('共 {{n}} 条', { n: total })}</Text>
            <Button
              disabled={params.page <= 1}
              onClick={() => setParams({ ...params, page: params.page - 1 })}
            >
              {t('上一页')}
            </Button>
            <Text>
              {params.page} / {Math.max(1, Math.ceil(total / (params.page_size || 12)))}
            </Text>
            <Button
              disabled={params.page * (params.page_size || 12) >= total}
              onClick={() => setParams({ ...params, page: params.page + 1 })}
            >
              {t('下一页')}
            </Button>
          </Space>
        </div>
      </Card>

      <Modal
        title={t('新建工单')}
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void onCreate()}
        destroyOnHidden
        width={560}
      >
        <Form form={createForm} layout="vertical" initialValues={{ priority: 'normal', source: 'manual' }}>
          <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('请填写标题') }]}>
            <Input maxLength={200} placeholder={t('简要描述问题')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={4} placeholder={t('现象、复现步骤、客户诉求…')} />
          </Form.Item>
          <Space wrap style={{ width: '100%' }} size="middle">
            <Form.Item name="priority" label={t('优先级')} style={{ marginBottom: 0 }}>
              <Select options={priorityOptions} style={{ width: 120 }} />
            </Form.Item>
            <Form.Item name="source" label={t('来源')} style={{ marginBottom: 0 }}>
              <Select options={sourceOptions} style={{ width: 120 }} />
            </Form.Item>
            <Form.Item
              name="due_at"
              label={t('截止日')}
              extra={t('可留空，将按优先级自动套 SLA')}
              style={{ marginBottom: 0 }}
            >
              <DatePicker />
            </Form.Item>
          </Space>
          <Space wrap style={{ width: '100%', marginTop: 12 }} size="middle">
            <Form.Item name="contact_name" label={t('联系人')} style={{ marginBottom: 0 }}>
              <Input style={{ width: 140 }} />
            </Form.Item>
            <Form.Item name="contact_phone" label={t('电话')} style={{ marginBottom: 0 }}>
              <Input style={{ width: 140 }} />
            </Form.Item>
            <Form.Item name="customer_id" label={t('CRM 客户 ID')} style={{ marginBottom: 0 }}>
              <InputNumber min={0} style={{ width: 120 }} placeholder={t('可选')} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      <TicketDetailDrawer
        open={detailOpen}
        ticketId={detailId && detailId > 0 ? detailId : null}
        onClose={closeDetail}
        onChanged={() => {
          void fetchList(params)
          void fetchStats()
        }}
      />
    </div>
  )
}
