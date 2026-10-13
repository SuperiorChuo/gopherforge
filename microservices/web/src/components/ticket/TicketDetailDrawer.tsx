/**
 * 可复用工单详情抽屉（工单中心 / IM 客户 360 共用）。
 * 传入 ticketId 打开；关闭时 ticketId 置空。
 */
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Descriptions,
  Drawer,
  Input,
  List,
  Select,
  Space,
  Tag,
  Timeline,
  Typography,
} from 'antd'
import {
  CheckOutlined,
  CloseOutlined,
  CommentOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  addTicketComment,
  assignTicket,
  changeTicketStatus,
  getTicket,
  updateTicket,
  PRIORITY_META,
  SOURCE_META,
  STATUS_META,
  type Ticket,
  type TicketEvent,
} from '@/api/ticket'
import { formatDateTime } from '@/utils/format'
import dayjs from 'dayjs'

const { Text, Paragraph } = Typography

const PRIORITY_OPTIONS = Object.entries(PRIORITY_META).map(([value, m]) => ({ value, label: m.label }))

export function isTicketOverdue(t: Pick<Ticket, 'due_at' | 'status'>) {
  if (!t.due_at) return false
  if (t.status !== 'open' && t.status !== 'in_progress') return false
  return dayjs(t.due_at).isBefore(dayjs())
}

function nextActions(status: string): { status: string; label: string; danger?: boolean; icon: React.ReactNode }[] {
  switch (status) {
    case 'open':
      return [
        { status: 'in_progress', label: '开始处理', icon: <PlayCircleOutlined /> },
        { status: 'resolved', label: '标记已解决', icon: <CheckOutlined /> },
        { status: 'closed', label: '直接关闭', danger: true, icon: <CloseOutlined /> },
      ]
    case 'in_progress':
      return [
        { status: 'resolved', label: '标记已解决', icon: <CheckOutlined /> },
        { status: 'closed', label: '关闭', danger: true, icon: <CloseOutlined /> },
      ]
    case 'resolved':
      return [
        { status: 'closed', label: '关闭', icon: <CloseOutlined /> },
        { status: 'in_progress', label: '重新打开', icon: <PlayCircleOutlined /> },
      ]
    case 'closed':
      return [{ status: 'open', label: '重新打开', icon: <PlayCircleOutlined /> }]
    default:
      return []
  }
}

export type TicketDetailDrawerProps = {
  ticketId: number | null
  open: boolean
  onClose: () => void
  /** 状态/评论/保存后回调（刷新列表/360） */
  onChanged?: () => void
  width?: number
}

export default function TicketDetailDrawer({
  ticketId,
  open,
  onClose,
  onChanged,
  width = 560,
}: TicketDetailDrawerProps) {
  const { t } = useTranslation()
  const [detail, setDetail] = useState<Ticket | null>(null)
  const [events, setEvents] = useState<TicketEvent[]>([])
  const [loading, setLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [comment, setComment] = useState('')
  const [statusNote, setStatusNote] = useState('')

  async function load(id: number) {
    setLoading(true)
    setComment('')
    setStatusNote('')
    try {
      const res = await getTicket(id)
      setDetail(res.ticket)
      setEvents(res.events ?? [])
    } catch {
      message.error(t('加载工单详情失败'))
      onClose()
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (open && ticketId) {
      void load(ticketId)
    }
    if (!open) {
      setDetail(null)
      setEvents([])
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, ticketId])

  async function refresh(id: number) {
    const res = await getTicket(id)
    setDetail(res.ticket)
    setEvents(res.events ?? [])
    onChanged?.()
  }

  async function onStatus(to: string) {
    if (!detail) return
    setActionLoading(true)
    try {
      await changeTicketStatus(detail.id, to, statusNote)
      message.success(t('状态已更新'))
      setStatusNote('')
      await refresh(detail.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('状态更新失败'))
    } finally {
      setActionLoading(false)
    }
  }

  async function onAssignSelf() {
    if (!detail) return
    setActionLoading(true)
    try {
      const { store } = await import('@/store')
      const me = store.getState().auth.userInfo?.id
      if (!me) {
        message.error(t('未登录'))
        return
      }
      await assignTicket(detail.id, me)
      message.success(t('已认领'))
      await refresh(detail.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('认领失败'))
    } finally {
      setActionLoading(false)
    }
  }

  async function onComment() {
    if (!detail || !comment.trim()) return
    setActionLoading(true)
    try {
      await addTicketComment(detail.id, comment.trim())
      setComment('')
      message.success(t('已评论'))
      await refresh(detail.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('评论失败'))
    } finally {
      setActionLoading(false)
    }
  }

  async function onSaveMeta() {
    if (!detail) return
    setActionLoading(true)
    try {
      await updateTicket(detail.id, {
        title: detail.title,
        description: detail.description,
        priority: detail.priority,
        contact_name: detail.contact_name,
        contact_phone: detail.contact_phone,
      })
      message.success(t('已保存'))
      await refresh(detail.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      setActionLoading(false)
    }
  }

  return (
    <Drawer
      title={detail ? `${detail.ticket_no} · ${detail.title}` : t('工单详情')}
      open={open}
      onClose={onClose}
      width={width}
      loading={loading}
      destroyOnHidden
      extra={
        detail ? (
          <Space>
            <Button size="small" onClick={() => void onAssignSelf()} loading={actionLoading}>
              {t('认领给我')}
            </Button>
            <Button size="small" type="primary" onClick={() => void onSaveMeta()} loading={actionLoading}>
              {t('保存')}
            </Button>
          </Space>
        ) : null
      }
    >
      {detail ? (
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Space wrap>
            <Tag color={STATUS_META[detail.status]?.color}>{STATUS_META[detail.status]?.label ? t(STATUS_META[detail.status].label) : detail.status}</Tag>
            <Tag color={PRIORITY_META[detail.priority]?.color}>
              {PRIORITY_META[detail.priority]?.label ? t(PRIORITY_META[detail.priority].label) : detail.priority}
            </Tag>
            <Tag>{SOURCE_META[detail.source] ? t(SOURCE_META[detail.source]) : detail.source}</Tag>
            {isTicketOverdue(detail) ? <Tag color="red">{t('已超时')}</Tag> : null}
          </Space>

          <Descriptions column={1} size="small">
            <Descriptions.Item label={t('创建')}>
              {t('用户 #{{id}} · {{time}}', { id: detail.creator_user_id, time: formatDateTime(detail.created_at) })}
            </Descriptions.Item>
            <Descriptions.Item label={t('处理人')}>
              {detail.assignee_user_id ? t('用户 #{{id}}', { id: detail.assignee_user_id }) : t('未指派')}
            </Descriptions.Item>
            {detail.conversation_public_id ? (
              <Descriptions.Item label={t('IM 会话')}>
                <a href="/im/desk" target="_blank" rel="noreferrer">
                  {detail.conversation_public_id}
                </a>
              </Descriptions.Item>
            ) : null}
            {detail.customer_id ? (
              <Descriptions.Item label={t('CRM 客户')}>
                <a href={`/crm/customers?keyword=${detail.customer_id}`} target="_blank" rel="noreferrer">
                  #{detail.customer_id}
                </a>
              </Descriptions.Item>
            ) : null}
            {detail.due_at ? (
              <Descriptions.Item label={t('截止')}>
                <span style={{ color: isTicketOverdue(detail) ? 'var(--c-error)' : undefined }}>
                  {formatDateTime(detail.due_at)}
                  {isTicketOverdue(detail) ? t('· 已超时') : ''}
                </span>
              </Descriptions.Item>
            ) : null}
            {detail.resolve_note ? (
              <Descriptions.Item label={t('解决备注')}>{detail.resolve_note}</Descriptions.Item>
            ) : null}
          </Descriptions>

          <div>
            <Text type="secondary">{t('标题')}</Text>
            <Input
              value={detail.title}
              onChange={(e) => setDetail({ ...detail, title: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </div>
          <div>
            <Text type="secondary">{t('描述')}</Text>
            <Input.TextArea
              rows={4}
              value={detail.description || ''}
              onChange={(e) => setDetail({ ...detail, description: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </div>
          <Space wrap>
            <div>
              <Text type="secondary">{t('优先级')}</Text>
              <br />
              <Select
                value={detail.priority}
                options={PRIORITY_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
                style={{ width: 120, marginTop: 4 }}
                onChange={(v) => setDetail({ ...detail, priority: v })}
              />
            </div>
            <div>
              <Text type="secondary">{t('联系人')}</Text>
              <Input
                value={detail.contact_name || ''}
                onChange={(e) => setDetail({ ...detail, contact_name: e.target.value })}
                style={{ width: 120, marginTop: 4, display: 'block' }}
              />
            </div>
            <div>
              <Text type="secondary">{t('电话')}</Text>
              <Input
                value={detail.contact_phone || ''}
                onChange={(e) => setDetail({ ...detail, contact_phone: e.target.value })}
                style={{ width: 140, marginTop: 4, display: 'block' }}
              />
            </div>
          </Space>

          <div>
            <Text strong>{t('状态流转')}</Text>
            <Input.TextArea
              rows={2}
              placeholder={t('可选备注（解决/关闭时写入解决说明）')}
              value={statusNote}
              onChange={(e) => setStatusNote(e.target.value)}
              style={{ margin: '8px 0' }}
            />
            <Space wrap>
              {nextActions(detail.status).map((a) => (
                <Button
                  key={a.status}
                  danger={a.danger}
                  icon={a.icon}
                  loading={actionLoading}
                  onClick={() => void onStatus(a.status)}
                >
                  {t(a.label)}
                </Button>
              ))}
            </Space>
          </div>

          <div>
            <Text strong>
              <CommentOutlined /> {t('评论')}
            </Text>
            <Input.TextArea
              rows={2}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder={t('补充处理进展…')}
              style={{ margin: '8px 0' }}
            />
            <Button type="primary" ghost loading={actionLoading} onClick={() => void onComment()}>
              {t('发表评论')}
            </Button>
          </div>

          <div>
            <Text strong>{t('时间线')}</Text>
            {events.length === 0 ? (
              <Paragraph type="secondary">{t('暂无事件')}</Paragraph>
            ) : (
              <Timeline
                style={{ marginTop: 12 }}
                items={events.map((ev) => ({
                  children: (
                    <div>
                      <Space size={4} wrap>
                        <Tag>{ev.kind}</Tag>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {t('用户 #{{id}} · {{time}}', { id: ev.by_user_id, time: formatDateTime(ev.created_at) })}
                        </Text>
                      </Space>
                      <div>{ev.content}</div>
                    </div>
                  ),
                }))}
              />
            )}
          </div>
        </Space>
      ) : (
        <List loading={loading} />
      )}
    </Drawer>
  )
}
