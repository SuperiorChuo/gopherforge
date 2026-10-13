/**
 * 顶栏「我的未结工单」下拉：点击弹出我的待处理工单清单，与通知一样在
 * header 内展示而非跳转到页面。统计数据轮询 60s 不变。
 */
import { useCallback, useEffect, useState } from 'react'
import { Badge, Button, Popover, Tag, Tooltip } from 'antd'
import { FileTextOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { getTicketStats, listTickets, type Ticket, type TicketStats } from '@/api/ticket'
import { getToken } from '@/utils/request'
import { formatDateTime } from '@/utils/format'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'

const POLL_MS = 60_000
const PAGE_SIZE = 10

const PRIORITY_META: Record<string, { label: string; color: string }> = {
  urgent: { label: '紧急', color: 'red' },
  high: { label: '高', color: 'orange' },
  normal: { label: '普通', color: 'blue' },
  low: { label: '低', color: 'default' },
}

const STATUS_LABELS: Record<string, string> = {
  open: '待处理',
  in_progress: '处理中',
  resolved: '已解决',
  closed: '已关闭',
}

export type TicketMineCounts = {
  mineOpen: number
  mineOverdue: number
}

/** 供侧栏菜单装饰复用；与顶栏同一轮询源 */
let latestCounts: TicketMineCounts = { mineOpen: 0, mineOverdue: 0 }
const listeners = new Set<(c: TicketMineCounts) => void>()

function publish(c: TicketMineCounts) {
  latestCounts = c
  listeners.forEach((fn) => fn(c))
}

export function getTicketMineCounts() {
  return latestCounts
}

export function subscribeTicketMineCounts(fn: (c: TicketMineCounts) => void) {
  listeners.add(fn)
  fn(latestCounts)
  return () => {
    listeners.delete(fn)
  }
}

export default function TicketMineBadge() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [stats, setStats] = useState<TicketStats | null>(null)
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [open, setOpen] = useState(false)

  const refresh = useCallback(async () => {
    if (!getToken()) return
    try {
      const st = await getTicketStats()
      setStats(st)
      publish({ mineOpen: st.mine_open ?? 0, mineOverdue: st.mine_overdue ?? 0 })
    } catch {
      /* 服务未起时静默 */
    }
  }, [])

  const loadMineTickets = useCallback(async () => {
    if (!getToken()) return
    try {
      const res = await listTickets({ scope: 'mine', status: 'open', page: 1, page_size: PAGE_SIZE })
      // listTickets 返回 { list, total }，内联 any 拆
      const data = res as unknown as { list?: Ticket[] }
      setTickets(data.list ?? [])
    } catch {
      /* silent */
    }
  }, [])

  useEffect(() => {
    void refresh()
    const t = window.setInterval(() => void refresh(), POLL_MS)
    const onFocus = () => void refresh()
    window.addEventListener('focus', onFocus)
    const onCustom = () => void refresh()
    window.addEventListener('ticket-stats-refresh', onCustom)
    return () => {
      window.clearInterval(t)
      window.removeEventListener('focus', onFocus)
      window.removeEventListener('ticket-stats-refresh', onCustom)
    }
  }, [refresh])

  const mineOpen = stats?.mine_open ?? 0
  const mineOverdue = stats?.mine_overdue ?? 0
  const badgeTitle =
    mineOpen > 0
      ? mineOverdue > 0
        ? `我的未结 ${mineOpen}（超时 ${mineOverdue}）`
        : `我的未结 ${mineOpen}`
      : '我的未结工单'

  const hasOverdue = mineOverdue > 0

  const handleOpenChange = (visible: boolean) => {
    setOpen(visible)
    if (visible) {
      void refresh()
      void loadMineTickets()
    }
  }

  const handleClickItem = () => {
    setOpen(false)
    navigate(`/ticket?scope=mine`)
  }

  const content = (
    <div className="notice-bell-panel" style={{ minWidth: 340, maxWidth: 420 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8, gap: 8 }}>
        <Button
          type="link"
          size="small"
          onClick={() => {
            setOpen(false)
            navigate('/ticket?scope=mine')
          }}
        >
          {t('查看全部')}
        </Button>
        <Button type="link" size="small" onClick={() => void loadMineTickets()}>
          {t('刷新')}
        </Button>
      </div>
      {tickets.length === 0 ? (
        <GlassEmpty text="暂无待处理工单" compact />
      ) : (
        tickets.map((tk) => {
          const priority = PRIORITY_META[tk.priority] ?? PRIORITY_META.normal
          return (
            <div
              className="notice-bell-item"
              key={tk.id}
              role="button"
              style={{ cursor: 'pointer' }}
              onClick={() => handleClickItem()}
            >
              <div className="notice-bell-item-head">
                <Tag color={priority.color} variant="filled" style={{ marginInlineEnd: 0 }}>
                  {t(priority.label)}
                </Tag>
                <StatusPill
                  tone={
                    tk.status === 'open' ? 'info'
                    : tk.status === 'in_progress' ? 'warning'
                    : 'muted'
                  }
                  label={STATUS_LABELS[tk.status] || tk.status}
                />
              </div>
              <span className="notice-bell-item-title">
                {tk.ticket_no && <span className="cell-mono" style={{ marginRight: 6 }}>#{tk.ticket_no}</span>}
                {tk.title}
              </span>
              {tk.due_at && (
                <div className="notice-bell-item-time">
                  {t('截止')}: {formatDateTime(tk.due_at)}
                </div>
              )}
            </div>
          )
        })
      )}
    </div>
  )

  return (
    <Popover
      content={content}
      title={t('我的未结工单')}
      rootClassName="notice-bell-popover"
      trigger="click"
      placement="bottomRight"
      open={open}
      onOpenChange={handleOpenChange}
    >
      <Tooltip title={open ? '' : badgeTitle}>
        <span className="app-trigger" style={{ position: 'relative' }}>
          <Badge
            count={mineOpen}
            size="small"
            overflowCount={99}
            color={hasOverdue ? 'var(--c-error)' : undefined}
            offset={[2, -2]}
          >
            <FileTextOutlined style={{ fontSize: 16 }} />
          </Badge>
        </span>
      </Tooltip>
    </Popover>
  )
}

/** 通知全局角标刷新（工单页 CRUD 后调用） */
export function notifyTicketStatsRefresh() {
  window.dispatchEvent(new Event('ticket-stats-refresh'))
}
