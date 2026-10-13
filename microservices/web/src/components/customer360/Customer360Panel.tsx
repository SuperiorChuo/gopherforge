/**
 * 客户 360：CRM 识客 + 未结工单 + IM 历史 + 近通话 + 快捷动作。
 * 支持两种入口：
 * - IM：传 conversation（按会话访客聚合）
 * - 电话/CC：传 phone（按号码聚合，呼叫工作台 / 弹屏）
 */
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Empty, Form, Input, Modal, Select, Space, Spin, Tag, Tooltip, Typography } from 'antd'
import {
  FileAddOutlined,
  IdcardOutlined,
  PhoneOutlined,
  ReloadOutlined,
  UserAddOutlined,
} from '@ant-design/icons'
import { createLead as createCrmLead, lookupPhone, type PhoneLookup } from '@/api/crm'
import { lookupCcNumber, type CcLookup } from '@/api/cc'
import {
  getConversationVisitor,
  getVisitorHistory,
  listVisitorConversations,
  type ImConversation,
  type ImVisitorCard,
} from '@/api/im'
import { createTicket, listTickets, STATUS_META, PRIORITY_META, type Ticket } from '@/api/ticket'
import TicketDetailDrawer, { isTicketOverdue } from '@/components/ticket/TicketDetailDrawer'
import { message } from '@/utils/feedback'
import { formatDateTime } from '@/utils/format'

const { Text, Paragraph } = Typography

export type Customer360PanelProps = {
  /** IM 模式：当前会话 */
  conversation?: ImConversation | null
  /** 电话模式：主叫/被叫号码 */
  phone?: string | null
  /** 电话模式可选展示名 */
  displayName?: string
  refreshToken?: number
  onJumpConversation?: (publicId: string) => void
  /** IM 模式：由外层打开转工单弹窗；电话模式可省略（内置建单） */
  onCreateTicket?: () => void
  onConvertLead?: () => void
  onEditVisitor?: () => void
  /** 紧凑高度（CC 卡片内嵌） */
  compact?: boolean
  className?: string
}

type LoadState = {
  loading: boolean
  visitor: ImVisitorCard | null
  crm: PhoneLookup | null
  tickets: Ticket[]
  history: ImConversation[]
  calls: CcLookup | null
  resolvedPhone: string
  resolvedName: string
  errors: string[]
}

const emptyState = (): LoadState => ({
  loading: false,
  visitor: null,
  crm: null,
  tickets: [],
  history: [],
  calls: null,
  resolvedPhone: '',
  resolvedName: '',
  errors: [],
})

function openTicket(t: Ticket) {
  return t.status === 'open' || t.status === 'in_progress'
}

async function collectTickets(opts: {
  conversationPublicId?: string
  phone?: string
  customerId?: number
}): Promise<Ticket[]> {
  let tickets: Ticket[] = []
  if (opts.conversationPublicId) {
    try {
      const byConv = await listTickets({
        conversation_public_id: opts.conversationPublicId,
        page: 1,
        page_size: 20,
      })
      tickets = byConv.list || []
    } catch {
      /* ignore */
    }
  }
  const merge = (more: Ticket[]) => {
    const seen = new Set(tickets.map((t) => t.id))
    for (const t of more) {
      if (!seen.has(t.id)) {
        seen.add(t.id)
        tickets.push(t)
      }
    }
  }
  if (opts.phone) {
    try {
      for (const st of ['open', 'in_progress'] as const) {
        const res = await listTickets({ keyword: opts.phone, status: st, page: 1, page_size: 10 })
        merge(res.list || [])
      }
    } catch {
      /* ignore */
    }
  }
  if (opts.customerId && opts.customerId > 0) {
    try {
      for (const st of ['open', 'in_progress'] as const) {
        const res = await listTickets({ customer_id: opts.customerId, status: st, page: 1, page_size: 10 })
        merge(res.list || [])
      }
    } catch {
      /* ignore */
    }
  }
  tickets.sort((a, b) => {
    const ao = openTicket(a) ? 0 : 1
    const bo = openTicket(b) ? 0 : 1
    if (ao !== bo) return ao - bo
    return b.id - a.id
  })
  return tickets
}

export default function Customer360Panel({
  conversation = null,
  phone = null,
  displayName,
  refreshToken = 0,
  onJumpConversation,
  onCreateTicket,
  onConvertLead,
  onEditVisitor,
  compact = false,
  className,
}: Customer360PanelProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<LoadState>(emptyState)
  const [drawerTicketId, setDrawerTicketId] = useState<number | null>(null)
  const [ticketOpen, setTicketOpen] = useState(false)
  const [ticketForm] = Form.useForm()
  const [creatingTicket, setCreatingTicket] = useState(false)
  const [creatingLead, setCreatingLead] = useState(false)

  const mode: 'im' | 'phone' | 'empty' = conversation?.public_id
    ? 'im'
    : phone && phone.trim()
      ? 'phone'
      : 'empty'

  const loadIm = useCallback(async (publicId: string) => {
    setState((s) => ({ ...emptyState(), loading: true, errors: s.errors }))
    const errors: string[] = []
    let visitor: ImVisitorCard | null = null
    try {
      visitor = await getConversationVisitor(publicId)
    } catch {
      errors.push(t('访客'))
    }
    let history: ImConversation[] = []
    try {
      const hist = await getVisitorHistory(publicId, 8)
      history = hist.list || []
    } catch {
      errors.push(t('历史会话'))
    }
    const resolvedPhone = (visitor?.visitor.phone || '').trim()
    let crm: PhoneLookup | null = null
    let calls: CcLookup | null = null
    if (resolvedPhone) {
      try {
        crm = await lookupPhone(resolvedPhone)
      } catch {
        errors.push(t('CRM'))
      }
      try {
        calls = await lookupCcNumber(resolvedPhone)
      } catch {
        /* optional */
      }
    }
    const customerId = crm?.matched === 'customer' && crm.customer ? crm.customer.id : 0
    const tickets = await collectTickets({
      conversationPublicId: publicId,
      phone: resolvedPhone,
      customerId,
    })
    setState({
      loading: false,
      visitor,
      crm,
      tickets,
      history,
      calls,
      resolvedPhone,
      resolvedName: visitor?.visitor.display_name || '',
      errors,
    })
  }, [t])

  const loadPhone = useCallback(async (rawPhone: string, nameHint?: string) => {
    const p = rawPhone.trim()
    setState((s) => ({ ...emptyState(), loading: true, errors: s.errors, resolvedPhone: p }))
    const errors: string[] = []
    let crm: PhoneLookup | null = null
    let calls: CcLookup | null = null
    try {
      crm = await lookupPhone(p)
    } catch {
      errors.push(t('CRM'))
    }
    try {
      calls = await lookupCcNumber(p)
    } catch {
      /* optional */
    }
    const customerId = crm?.matched === 'customer' && crm.customer ? crm.customer.id : 0
    const tickets = await collectTickets({ phone: p, customerId })

    // IM 历史：客户上若绑了 im_visitor_id 则拉会话
    let history: ImConversation[] = []
    const imVisitorId = crm?.customer?.im_visitor_id
    if (imVisitorId && imVisitorId > 0) {
      try {
        const res = await listVisitorConversations(imVisitorId, 8)
        history = res.list || []
      } catch {
        /* ignore */
      }
    }

    const resolvedName =
      nameHint ||
      crm?.customer?.name ||
      crm?.lead?.name ||
      calls?.contact?.name ||
      p

    setState({
      loading: false,
      visitor: null,
      crm,
      tickets,
      history,
      calls,
      resolvedPhone: p,
      resolvedName,
      errors,
    })
  }, [t])

  useEffect(() => {
    if (mode === 'im' && conversation?.public_id) {
      void loadIm(conversation.public_id)
      return
    }
    if (mode === 'phone' && phone) {
      void loadPhone(phone, displayName)
      return
    }
    setState(emptyState())
  }, [mode, conversation?.public_id, phone, displayName, refreshToken, loadIm, loadPhone])

  function reload() {
    if (mode === 'im' && conversation?.public_id) void loadIm(conversation.public_id)
    else if (mode === 'phone' && phone) void loadPhone(phone, displayName)
  }

  async function handleCreateTicketBuiltin() {
    const values = await ticketForm.validateFields()
    setCreatingTicket(true)
    try {
      const tk = await createTicket({
        title: values.title,
        description: values.description,
        priority: values.priority || 'normal',
        source: mode === 'im' ? 'im' : 'call',
        conversation_public_id: conversation?.public_id,
        im_visitor_id: state.visitor?.visitor.id,
        contact_name: state.resolvedName || undefined,
        contact_phone: state.resolvedPhone || undefined,
        customer_id:
          state.crm?.matched === 'customer' && state.crm.customer ? state.crm.customer.id : undefined,
      })
      message.success(t('已创建 {{no}}', { no: tk.ticket_no }))
      setTicketOpen(false)
      ticketForm.resetFields()
      setDrawerTicketId(tk.id)
      reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建失败'))
    } finally {
      setCreatingTicket(false)
    }
  }

  function openCreateTicket() {
    if (onCreateTicket) {
      onCreateTicket()
      return
    }
    const name = state.resolvedName || state.resolvedPhone || t('客户')
    ticketForm.setFieldsValue({
      title: mode === 'phone' ? t('来电跟进：{{name}}', { name }) : t('IM 咨询：{{name}}', { name }),
      description: '',
      priority: 'normal',
    })
    setTicketOpen(true)
  }

  async function handleConvertLead() {
    if (onConvertLead) {
      onConvertLead()
      return
    }
    const p = state.resolvedPhone
    setCreatingLead(true)
    try {
      if (p) {
        const hit = await lookupPhone(p)
        if (hit.matched === 'customer' && hit.customer) {
          message.info(t('已是客户「{{name}}」', { name: hit.customer.name }))
          window.open(`/crm/customers?keyword=${encodeURIComponent(p)}`, '_blank')
          return
        }
        if (hit.matched === 'lead' && hit.lead) {
          message.info(t('已有线索「{{name}}」', { name: hit.lead.name }))
          window.open(`/crm/leads?keyword=${encodeURIComponent(p)}`, '_blank')
          return
        }
      }
      const name = state.resolvedName || (p ? t('来电 {{number}}', { number: p }) : t('新线索'))
      await createCrmLead({
        name,
        phone: p || '',
        source: mode === 'im' ? 'im' : 'call',
        im_visitor_id: state.visitor?.visitor.id,
      })
      message.success(t('已转入销售线索'))
      reload()
      window.open(`/crm/leads?keyword=${encodeURIComponent(p || name)}`, '_blank')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('转入失败'))
    } finally {
      setCreatingLead(false)
    }
  }

  if (mode === 'empty') {
    return (
      <div className={`im-desk-360${compact ? ' is-compact' : ''} ${className || ''}`}>
        <div className="im-desk-360-head">
          <Text strong>{t('客户 360')}</Text>
        </div>
        <div className="im-desk-360-body">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={conversation === null && !phone ? t('选择会话或输入号码后展示') : t('暂无客户上下文')}
          />
        </div>
      </div>
    )
  }

  const v = state.visitor?.visitor
  const phoneShow = state.resolvedPhone
  const openTickets = state.tickets.filter(openTicket)
  const closedTickets = state.tickets.filter((t) => !openTicket(t)).slice(0, 3)
  const recentCalls = state.calls?.recent_calls || []
  const displayTitle =
    state.resolvedName ||
    v?.display_name ||
    (conversation ? t('访客 #{{id}}', { id: v?.id || conversation.visitor_id || '—' }) : phoneShow || t('客户'))

  return (
    <div className={`im-desk-360${compact ? ' is-compact' : ''} ${className || ''}`}>
      <div className="im-desk-360-head">
        <Text strong>{t('客户 360')}</Text>
        <Space size={4}>
          <Tooltip title={t('刷新')}>
            <Button
              size="small"
              type="text"
              icon={<ReloadOutlined spin={state.loading} />}
              onClick={() => reload()}
            />
          </Tooltip>
        </Space>
      </div>

      <div className="im-desk-360-body">
        {state.loading && !state.crm && !state.visitor ? (
          <div style={{ textAlign: 'center', padding: 24 }}>
            <Spin size="small" />
          </div>
        ) : (
          <Space direction="vertical" size={14} style={{ width: '100%' }}>
            <section className="im-desk-360-sec">
              <div className="im-desk-360-sec-title">{t('身份')}</div>
              <div className="im-desk-360-name">{displayTitle}</div>
              <Space direction="vertical" size={2} style={{ width: '100%' }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {t('电话')} {phoneShow || t('未留下')}
                  {v?.email ? ` · ${v.email}` : ''}
                </Text>
                {v?.wechat ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {t('微信')} {v.wechat}
                  </Text>
                ) : null}
                {v?.note ? (
                  <Paragraph type="secondary" ellipsis={{ rows: 2 }} style={{ marginBottom: 0, fontSize: 12 }}>
                    {t('备注：{{note}}', { note: v.note })}
                  </Paragraph>
                ) : null}
                {state.calls?.contact?.remark ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {t('CC 备注：{{note}}', { note: state.calls.contact.remark })}
                  </Text>
                ) : null}
              </Space>
              <div style={{ marginTop: 8 }}>
                {state.crm?.matched === 'customer' && state.crm.customer ? (
                  <Tag
                    color="green"
                    style={{ cursor: 'pointer' }}
                    onClick={() =>
                      window.open(
                        `/crm/customers?keyword=${encodeURIComponent(state.crm!.customer!.phone || state.crm!.customer!.name)}`,
                        '_blank',
                      )
                    }
                  >
                    {t('CRM 客户 · {{name}}', { name: state.crm.customer.name })}
                    {state.crm.customer.level ? ` · ${state.crm.customer.level}` : ''}
                  </Tag>
                ) : state.crm?.matched === 'lead' && state.crm.lead ? (
                  <Tag
                    color="blue"
                    style={{ cursor: 'pointer' }}
                    onClick={() =>
                      window.open(
                        `/crm/leads?keyword=${encodeURIComponent(state.crm!.lead!.phone || state.crm!.lead!.name)}`,
                        '_blank',
                      )
                    }
                  >
                    {t('CRM 线索 · {{name}}', { name: state.crm.lead.name })}
                  </Tag>
                ) : phoneShow ? (
                  <Tag>{t('CRM 未建档')}</Tag>
                ) : (
                  <Tag>{t('无电话，无法识客')}</Tag>
                )}
              </div>
            </section>

            <section className="im-desk-360-sec">
              <div className="im-desk-360-sec-title">{t('快捷动作')}</div>
              <Space wrap size={[6, 6]}>
                <Button size="small" icon={<FileAddOutlined />} onClick={() => openCreateTicket()}>
                  {t('转工单')}
                </Button>
                <Button
                  size="small"
                  icon={<UserAddOutlined />}
                  loading={creatingLead}
                  onClick={() => void handleConvertLead()}
                >
                  {t('转线索')}
                </Button>
                {mode === 'im' && onEditVisitor ? (
                  <Button size="small" icon={<IdcardOutlined />} onClick={onEditVisitor}>
                    {t('编辑访客')}
                  </Button>
                ) : null}
                {mode === 'phone' && phoneShow ? (
                  <Button
                    size="small"
                    icon={<PhoneOutlined />}
                    onClick={() => window.open(`/cc/calls?number=${encodeURIComponent(phoneShow)}`, '_blank')}
                  >
                    {t('话单')}
                  </Button>
                ) : null}
                {state.crm?.matched === 'customer' && state.crm.customer ? (
                  <Button
                    size="small"
                    onClick={() =>
                      window.open(
                        `/crm/customers?keyword=${encodeURIComponent(state.crm!.customer!.phone || String(state.crm!.customer!.id))}`,
                        '_blank',
                      )
                    }
                  >
                    {t('打开客户')}
                  </Button>
                ) : null}
                {openTickets[0] ? (
                  <Button size="small" onClick={() => setDrawerTicketId(openTickets[0].id)}>
                    {t('打开工单')}
                  </Button>
                ) : null}
              </Space>
            </section>

            <section className="im-desk-360-sec">
              <div className="im-desk-360-sec-title">
                {t('未结工单')}
                {openTickets.length > 0 ? (
                  <Tag color="orange" style={{ marginLeft: 6 }}>
                    {openTickets.length}
                  </Tag>
                ) : null}
              </div>
              {openTickets.length === 0 ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {t('暂无未结工单')}
                </Text>
              ) : (
                <Space direction="vertical" size={6} style={{ width: '100%' }}>
                  {openTickets.slice(0, 5).map((tk) => {
                    const sm = STATUS_META[tk.status] || { label: tk.status, color: 'default' }
                    const pm = PRIORITY_META[tk.priority] || { label: tk.priority, color: 'default' }
                    const overdue = isTicketOverdue(tk)
                    return (
                      <div key={tk.id} className="im-desk-360-row" onClick={() => setDrawerTicketId(tk.id)}>
                        <Space size={4} wrap>
                          <Text code style={{ fontSize: 11 }}>
                            {tk.ticket_no}
                          </Text>
                          <Tag color={sm.color}>{t(sm.label)}</Tag>
                          <Tag color={pm.color}>{t(pm.label)}</Tag>
                          {overdue ? <Tag color="red">{t('超时')}</Tag> : null}
                        </Space>
                        <div className="im-desk-360-row-title">{tk.title}</div>
                      </div>
                    )
                  })}
                </Space>
              )}
              {closedTickets.length > 0 ? (
                <div style={{ marginTop: 8 }}>
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    {t('最近已结')}
                  </Text>
                  {closedTickets.map((tk) => (
                    <div
                      key={tk.id}
                      className="im-desk-360-row is-muted"
                      onClick={() => setDrawerTicketId(tk.id)}
                    >
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {tk.ticket_no} · {STATUS_META[tk.status]?.label ? t(STATUS_META[tk.status].label) : tk.status} · {tk.title}
                      </Text>
                    </div>
                  ))}
                </div>
              ) : null}
            </section>

            <section className="im-desk-360-sec">
              <div className="im-desk-360-sec-title">{t('IM 历史')}</div>
              {state.history.length === 0 ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {mode === 'phone' ? t('无关联 IM 访客或暂无会话') : t('暂无历史')}
                </Text>
              ) : (
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  {state.history.map((h) => (
                    <div
                      key={h.public_id}
                      className={`im-desk-360-row${
                        conversation?.public_id === h.public_id ? ' is-active' : ''
                      }`}
                      onClick={() => {
                        if (onJumpConversation) onJumpConversation(h.public_id)
                        else window.open('/im/desk', '_blank')
                      }}
                    >
                      <Space size={4} wrap>
                        <span className="cell-mono" style={{ fontSize: 11 }}>
                          {h.public_id.slice(0, 8)}
                        </span>
                        <Tag>{h.status}</Tag>
                      </Space>
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        {formatDateTime(h.created_at)}
                        {h.last_message_preview ? ` · ${h.last_message_preview.slice(0, 40)}` : ''}
                      </Text>
                    </div>
                  ))}
                </Space>
              )}
            </section>

            <section className="im-desk-360-sec">
              <div className="im-desk-360-sec-title">
                <PhoneOutlined /> {t('近通话')}
              </div>
              {!phoneShow ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {t('无电话号码，无法匹配话单')}
                </Text>
              ) : recentCalls.length === 0 ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {state.calls ? t('近 5 通无记录') : t('通话数据暂不可用')}
                </Text>
              ) : (
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  {recentCalls.map((c) => (
                    <div key={c.call_id} className="im-desk-360-row is-static">
                      <Space size={4} wrap>
                        <Tag color={c.direction === 'inbound' ? 'blue' : 'cyan'}>
                          {c.direction === 'inbound' ? t('呼入') : c.direction === 'outbound' ? t('呼出') : c.direction}
                        </Tag>
                        <Tag>{c.status}</Tag>
                        <Text type="secondary" style={{ fontSize: 11 }}>
                          {c.duration_sec}s
                          {c.agent_ext ? t(' · 分机 {{ext}}', { ext: c.agent_ext }) : ''}
                        </Text>
                      </Space>
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        {c.started_at ? formatDateTime(c.started_at) : c.call_id.slice(0, 12)}
                      </Text>
                    </div>
                  ))}
                  <Button
                    type="link"
                    size="small"
                    style={{ padding: 0 }}
                    onClick={() => window.open(`/cc/calls?number=${encodeURIComponent(phoneShow)}`, '_blank')}
                  >
                    {t('查看全部话单')}
                  </Button>
                </Space>
              )}
            </section>

            {state.errors.length > 0 ? (
              <Text type="secondary" style={{ fontSize: 11 }}>
                {t('部分模块加载失败：{{modules}}', { modules: state.errors.join('、') })}
              </Text>
            ) : null}
          </Space>
        )}
      </div>

      <TicketDetailDrawer
        open={drawerTicketId != null}
        ticketId={drawerTicketId}
        onClose={() => setDrawerTicketId(null)}
        onChanged={() => reload()}
        width={520}
      />

      <Modal
        title={t('创建工单')}
        open={ticketOpen}
        onCancel={() => setTicketOpen(false)}
        onOk={() => void handleCreateTicketBuiltin()}
        confirmLoading={creatingTicket}
        destroyOnHidden
      >
        <Form form={ticketForm} layout="vertical">
          <Form.Item name="title" label={t('标题')} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="priority" label={t('优先级')} initialValue="normal">
            <Select
              options={[
                { value: 'low', label: t('低') },
                { value: 'normal', label: t('普通') },
                { value: 'high', label: t('高') },
                { value: 'urgent', label: t('紧急') },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
