// cc-service 实时话务事件通道（坐席浏览器弹屏底座）。
//
// 单例连接 /api/v1/cc/ws：先经 POST /api/v1/cc/ws/ticket 换取 60 秒一次性握手票据
// 再建立 WebSocket（不把长期 Access JWT 拼入 URL Query，兼容 HttpOnly Cookie 会话），
// 把话务事件 call.ringing/answered/hangup 广播给订阅者。多处订阅共享同一条连接（引用计数）。
import { createCcWsTicket } from '@/api/cc'

export interface CcCallEvent {
  type: 'call.ringing' | 'call.answered' | 'call.hangup'
  call_id: string
  direction: string
  caller: string
  callee: string
  agent_ext?: string
  queue?: string
  caller_name?: string
  bill_sec?: number
  ts: number
}

type Handler = (ev: CcCallEvent) => void

const MAX_RETRIES = 8
// 重连退避：2s / 4s / 8s / 16s / 30s（封顶）
const backoff = (attempt: number) => Math.min(2000 * 2 ** attempt, 30000)
const PING_INTERVAL = 25000

let ws: WebSocket | null = null
let connecting = false
let retries = 0
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let pingTimer: ReturnType<typeof setInterval> | null = null
const handlers = new Set<Handler>()

function clearTimers() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (pingTimer) {
    clearInterval(pingTimer)
    pingTimer = null
  }
}

function scheduleReconnect() {
  if (handlers.size === 0 || retries >= MAX_RETRIES) return
  if (reconnectTimer) return
  const delay = backoff(retries)
  retries += 1
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    void connect()
  }, delay)
}

async function connect() {
  if (import.meta.env.VITE_DEMO === '1' || handlers.size === 0 || connecting) return
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
  connecting = true
  let ticket = ''
  try {
    const res = await createCcWsTicket()
    ticket = res?.ticket || ''
  } catch {
    connecting = false
    scheduleReconnect()
    return
  }
  connecting = false
  if (!ticket || handlers.size === 0) return
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const sock = new WebSocket(
    `${proto}://${window.location.host}/api/v1/cc/ws?ticket=${encodeURIComponent(ticket)}`,
  )
  ws = sock

  sock.onopen = () => {
    retries = 0
    if (pingTimer) clearInterval(pingTimer)
    // 应用层心跳：给一些反代（Traefik 默认放行 WS，但空闲连接仍可能被中间设备回收）兜底
    pingTimer = setInterval(() => {
      if (sock.readyState === WebSocket.OPEN) {
        try {
          sock.send(JSON.stringify({ type: 'ping' }))
        } catch {
          /* ignore */
        }
      }
    }, PING_INTERVAL)
  }

  sock.onmessage = (event) => {
    let msg: Partial<CcCallEvent> | null = null
    try {
      msg = JSON.parse(event.data)
    } catch {
      return
    }
    if (!msg || typeof msg.type !== 'string') return
    if (msg.type === 'call.ringing' || msg.type === 'call.answered' || msg.type === 'call.hangup') {
      const ev = msg as CcCallEvent
      handlers.forEach((h) => {
        try {
          h(ev)
        } catch {
          /* 订阅者异常不影响其他订阅者 */
        }
      })
    }
  }

  sock.onclose = () => {
    if (ws === sock) ws = null
    if (pingTimer) {
      clearInterval(pingTimer)
      pingTimer = null
    }
    scheduleReconnect()
  }

  sock.onerror = () => {
    // onclose 会紧随其后触发重连，这里不重复安排
    try {
      sock.close()
    } catch {
      /* ignore */
    }
  }
}

// subscribeCcEvents 注册一个话务事件回调，返回取消订阅函数。第一个订阅者建立连接，
// 最后一个取消订阅时断开连接（引用计数）。
export function subscribeCcEvents(cb: Handler): () => void {
  handlers.add(cb)
  void connect()
  return () => {
    handlers.delete(cb)
    if (handlers.size === 0) {
      clearTimers()
      retries = 0
      if (ws) {
        const sock = ws
        ws = null
        try {
          sock.close()
        } catch {
          /* ignore */
        }
      }
    }
  }
}
