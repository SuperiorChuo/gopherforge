import { useCallback, useEffect, useRef, useState } from 'react'
import { Badge, Button, Popover, Space, Tag, Tooltip } from 'antd'
import { BellOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { createNotificationTicket } from '@/api/system/notice'
import {
  getNotifyUnreadCount,
  listNotifyMessages,
  markAllNotifyRead,
  markNotifyRead,
  type NotifyMessage,
} from '@/api/notify'
import { formatDateTime } from '@/utils/format'
import GlassEmpty from '@/components/common/GlassEmpty'

interface NotificationItem {
  id: string
  type: string
  title: string
  content: string
  link?: string
  created_at: string
  /** true when from notify-service inbox (can mark read) */
  inbox?: boolean
  read?: boolean
}

const MAX_ITEMS = 20
const MAX_RETRIES = 5
const POLL_MS = 45_000
// 重连退避：5s / 10s / 20s / 40s / 60s
const backoff = (attempt: number) => Math.min(5000 * 2 ** attempt, 60000)

const typeLabels: Record<string, { text: string; color: string }> = {
  announcement: { text: '公告', color: 'orange' },
  notice: { text: '通知', color: 'blue' },
  ticket: { text: '工单', color: 'red' },
  im: { text: 'IM', color: 'green' },
  system: { text: '系统', color: 'default' },
}

function fromInbox(m: NotifyMessage): NotificationItem {
  return {
    id: `inbox-${m.id}`,
    type: m.type || 'notice',
    title: m.title,
    content: m.content,
    link: m.link,
    created_at: m.created_at,
    inbox: true,
    read: !!m.read_at,
  }
}

function mergeItems(ws: NotificationItem[], inbox: NotificationItem[]): NotificationItem[] {
  const map = new Map<string, NotificationItem>()
  // inbox first so WS replay doesn't clobber real messages
  for (const n of inbox) map.set(n.id, n)
  for (const n of ws) {
    if (!map.has(n.id)) map.set(n.id, n)
  }
  return Array.from(map.values())
    .sort((a, b) => {
      const ta = Date.parse(a.created_at) || 0
      const tb = Date.parse(b.created_at) || 0
      return tb - ta
    })
    .slice(0, MAX_ITEMS)
}

export default function NotificationBell() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [items, setItems] = useState<NotificationItem[]>([])
  const [announcementUnread, setAnnouncementUnread] = useState(0)
  const [inboxUnread, setInboxUnread] = useState(0)
  const unread = announcementUnread + inboxUnread
  const [ringing, setRinging] = useState(false)
  const [open, setOpen] = useState(false)

  const seenRef = useRef<Set<string>>(new Set())
  const wsItemsRef = useRef<NotificationItem[]>([])
  const inboxItemsRef = useRef<NotificationItem[]>([])
  const wsRef = useRef<WebSocket | null>(null)
  const retriesRef = useRef(0)
  const replayUntilRef = useRef(0)
  const unmountedRef = useRef(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const prevInboxUnread = useRef(0)

  const applyMerge = useCallback(() => {
    setItems(mergeItems(wsItemsRef.current, inboxItemsRef.current))
  }, [])

  const loadInbox = useCallback(async (opts?: { ring?: boolean }) => {
    if (unmountedRef.current) return
    try {
      const [listRes, countRes] = await Promise.all([
        listNotifyMessages({ page: 1, page_size: 20 }),
        getNotifyUnreadCount(),
      ])
      if (unmountedRef.current) return
      const inbox = (listRes?.list ?? []).map(fromInbox)
      inboxItemsRef.current = inbox
      for (const n of inbox) seenRef.current.add(n.id)
      applyMerge()
      const count = Number(countRes?.count ?? 0)
      setInboxUnread(count)
      if (opts?.ring && count > prevInboxUnread.current) {
        setRinging(true)
      }
      prevInboxUnread.current = count
    } catch {
      // notify-service 未就绪时静默（仍保留 WS 公告）
    }
  }, [applyMerge])

  const connect = useCallback(async () => {
    // The static Demo has no WebSocket backend; its inbox uses the mock adapter.
    if (import.meta.env.VITE_DEMO === '1' || unmountedRef.current) return
    try {
      const { ticket } = await createNotificationTicket()
      if (unmountedRef.current) return
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(
        `${proto}://${window.location.host}/api/v1/ws/notifications?ticket=${encodeURIComponent(ticket)}`,
      )
      wsRef.current = ws

      ws.onopen = () => {
        retriesRef.current = 0
        // 建连后服务端会回放当前生效的公告，这段时间的消息不计未读
        replayUntilRef.current = Date.now() + 1500
      }

      ws.onmessage = (event) => {
        let msg: NotificationItem | null = null
        try {
          msg = JSON.parse(event.data)
        } catch {
          return
        }
        if (!msg?.id || seenRef.current.has(msg.id)) return
        seenRef.current.add(msg.id)
        const next = { ...msg, inbox: false }
        wsItemsRef.current = [next, ...wsItemsRef.current].slice(0, MAX_ITEMS)
        applyMerge()
        if (Date.now() > replayUntilRef.current) {
          // 公告未读与站内信未读分开：公告只在铃铛角标上 +1，打开面板清零本地
          setAnnouncementUnread((n) => n + 1)
          setRinging(true)
        }
      }

      ws.onclose = () => {
        wsRef.current = null
        scheduleReconnect()
      }
    } catch {
      scheduleReconnect()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [applyMerge])

  const scheduleReconnect = useCallback(() => {
    if (unmountedRef.current || retriesRef.current >= MAX_RETRIES) return
    const delay = backoff(retriesRef.current)
    retriesRef.current += 1
    timerRef.current = setTimeout(connect, delay)
  }, [connect])

  useEffect(() => {
    unmountedRef.current = false
    connect()
    void loadInbox()

    // 铃铛挂在 MainLayout 上、全站常驻，后台标签页里的轮询纯属白打：
    // 挂后台一小时 = 80 次无人看的 /inbox。隐藏即停表，回前台补拉一次再续。
    const startPoll = () => {
      if (pollRef.current) clearInterval(pollRef.current)
      pollRef.current = setInterval(() => void loadInbox({ ring: true }), POLL_MS)
    }
    const stopPoll = () => {
      if (pollRef.current) clearInterval(pollRef.current)
      pollRef.current = null
    }
    startPoll()

    // 长时间休眠/切后台后重试次数可能耗尽，页面回到前台时若连接已死则复活
    const onVisible = () => {
      if (document.visibilityState !== 'visible') {
        stopPoll()
        return
      }
      startPoll()
      void loadInbox({ ring: true })
      const ws = wsRef.current
      if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
        retriesRef.current = 0
        if (timerRef.current) clearTimeout(timerRef.current)
        connect()
      }
    }
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      unmountedRef.current = true
      document.removeEventListener('visibilitychange', onVisible)
      if (timerRef.current) clearTimeout(timerRef.current)
      if (pollRef.current) clearInterval(pollRef.current)
      wsRef.current?.close()
    }
  }, [connect, loadInbox])

  const handleOpenChange = (visible: boolean) => {
    setOpen(visible)
    if (visible) {
      // 本地角标先清；站内信真正已读走「全部已读」或点开条目
      setAnnouncementUnread(0)
      void loadInbox()
    }
  }

  const handleMarkAll = async () => {
    try {
      await markAllNotifyRead()
      await loadInbox()
    } catch {
      /* silent */
    }
  }

  const handleClickItem = async (n: NotificationItem) => {
    if (n.inbox) {
      const rawId = Number(String(n.id).replace(/^inbox-/, ''))
      if (rawId > 0 && !n.read) {
        try {
          await markNotifyRead(rawId)
          await loadInbox()
        } catch {
          /* silent */
        }
      }
    }
    if (n.link) {
      setOpen(false)
      if (n.link.startsWith('http://') || n.link.startsWith('https://')) {
        window.open(n.link, '_blank', 'noopener,noreferrer')
      } else {
        navigate(n.link)
      }
    }
  }

  const content = (
    <div className="notice-bell-panel">
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8, gap: 8 }}>
        <Button
          type="link"
          size="small"
          onClick={() => {
            setOpen(false)
            navigate('/system/inbox')
          }}
        >
          {t('查看全部')}
        </Button>
        <Space size={4}>
          <Button type="link" size="small" onClick={() => void loadInbox()}>
            {t('刷新')}
          </Button>
          <Button type="link" size="small" onClick={() => void handleMarkAll()}>
            {t('全部已读')}
          </Button>
        </Space>
      </div>
      {items.length === 0 ? (
        <GlassEmpty text="暂无通知" compact />
      ) : (
        items.map((n) => {
          const meta = typeLabels[n.type] ?? typeLabels.notice
          return (
            <div
              className="notice-bell-item"
              key={n.id}
              role={n.link ? 'button' : undefined}
              style={{
                cursor: n.link ? 'pointer' : undefined,
                opacity: n.read ? 0.72 : 1,
              }}
              onClick={() => void handleClickItem(n)}
            >
              <div className="notice-bell-item-head">
                <Tag color={meta.color} variant="filled" style={{ marginInlineEnd: 0 }}>
                  {t(meta.text)}
                </Tag>
                <span className="notice-bell-item-title">{n.title}</span>
              </div>
              {n.content && <div className="notice-bell-item-content">{n.content}</div>}
              <div className="notice-bell-item-time">{formatDateTime(n.created_at)}</div>
            </div>
          )
        })
      )}
    </div>
  )

  return (
    <Popover
      content={content}
      title={t('通知')}
      rootClassName="notice-bell-popover"
      trigger="click"
      placement="bottomRight"
      open={open}
      onOpenChange={handleOpenChange}
    >
      <Tooltip title={open ? '' : t('通知')}>
        <span className="app-trigger">
          <Badge count={unread} size="small" offset={[2, -2]}>
            <BellOutlined
              style={{ fontSize: 17 }}
              className={ringing ? 'bell-ringing' : undefined}
              onAnimationEnd={() => setRinging(false)}
            />
          </Badge>
        </span>
      </Tooltip>
    </Popover>
  )
}
