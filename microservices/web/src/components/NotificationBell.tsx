import { useCallback, useEffect, useRef, useState } from 'react'
import { Badge, Popover, Tag, Tooltip } from 'antd'
import { BellOutlined } from '@ant-design/icons'
import { createNotificationTicket } from '@/api/system/notice'
import { formatDateTime } from '@/utils/format'
import GlassEmpty from '@/components/GlassEmpty'

interface NotificationItem {
  id: string
  type: string
  title: string
  content: string
  link?: string
  created_at: string
}

const MAX_ITEMS = 20
const MAX_RETRIES = 5
// 重连退避：5s / 10s / 20s / 40s / 60s
const backoff = (attempt: number) => Math.min(5000 * 2 ** attempt, 60000)

const typeLabels: Record<string, { text: string; color: string }> = {
  announcement: { text: '公告', color: 'orange' },
  notice: { text: '通知', color: 'blue' },
}

export default function NotificationBell() {
  const [items, setItems] = useState<NotificationItem[]>([])
  const [unread, setUnread] = useState(0)
  const [ringing, setRinging] = useState(false)
  const [open, setOpen] = useState(false)

  const seenRef = useRef<Set<string>>(new Set())
  const wsRef = useRef<WebSocket | null>(null)
  const retriesRef = useRef(0)
  const replayUntilRef = useRef(0)
  const unmountedRef = useRef(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(async () => {
    if (unmountedRef.current) return
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
        setItems((prev) => [msg as NotificationItem, ...prev].slice(0, MAX_ITEMS))
        if (Date.now() > replayUntilRef.current) {
          setUnread((n) => n + 1)
          // 新通知到达:摇铃一次(动画结束自动摘掉 class 以便下次重放)
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
  }, [])

  const scheduleReconnect = useCallback(() => {
    if (unmountedRef.current || retriesRef.current >= MAX_RETRIES) return
    const delay = backoff(retriesRef.current)
    retriesRef.current += 1
    timerRef.current = setTimeout(connect, delay)
  }, [connect])

  useEffect(() => {
    unmountedRef.current = false
    connect()

    // 长时间休眠/切后台后重试次数可能耗尽，页面回到前台时若连接已死则复活
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return
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
      wsRef.current?.close()
    }
  }, [connect])

  const handleOpenChange = (visible: boolean) => {
    setOpen(visible)
    if (visible) setUnread(0)
  }

  const content = (
    <div className="notice-bell-panel">
      {items.length === 0 ? (
        <GlassEmpty text="暂无通知" compact />
      ) : (
        items.map((n) => {
          const meta = typeLabels[n.type] ?? typeLabels.notice
          return (
            <div className="notice-bell-item" key={n.id}>
              <div className="notice-bell-item-head">
                <Tag color={meta.color} variant="filled" style={{ marginInlineEnd: 0 }}>
                  {meta.text}
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
      title="通知"
      trigger="click"
      placement="bottomRight"
      open={open}
      onOpenChange={handleOpenChange}
    >
      <Tooltip title={open ? '' : '通知'}>
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
