import { getActTenantId, getToken } from '@/utils/request'

export interface SseFrame<E> {
  id?: string
  event: string
  data: E
}

export interface SsePostOptions<E> {
  url: string
  body: unknown
  signal?: AbortSignal
  onEvent: (event: E) => void
}

export interface SseGetOptions<E> {
  url: string
  signal?: AbortSignal
  lastEventId?: string | number
  onEvent: (frame: SseFrame<E>) => void
}

function authHeaders(): Record<string, string> {
  if (import.meta.env.VITE_DEMO === '1') {
    throw new Error('Streaming is unavailable in the static Demo; deploy the backend to use this feature.')
  }
  const token = getToken()
  const actTenant = getActTenantId()
  return {
    Accept: 'text/event-stream',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(actTenant ? { 'X-Act-Tenant-ID': actTenant } : {}),
  }
}

async function responseError(response: Response): Promise<Error> {
  let text = `请求失败 (${response.status})`
  try {
    const data = await response.json() as { message?: unknown }
    if (typeof data.message === 'string' && data.message) text = data.message
  } catch {
    // 非 JSON 错误体保留稳定状态码，不把 HTML 等正文带到界面。
  }
  return new Error(text)
}

async function consumeSSE<E>(response: Response, onEvent: (frame: SseFrame<E>) => void): Promise<void> {
  if (!response.ok) throw await responseError(response)
  if (!response.body) throw new Error('当前环境不支持流式响应')

  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''

  const dispatch = (rawBlock: string) => {
    const dataLines: string[] = []
    let id: string | undefined
    let event = 'message'
    for (const line of rawBlock.split('\n')) {
      if (line.startsWith('data:')) dataLines.push(line.slice(5).replace(/^ /, ''))
      else if (line.startsWith('id:')) id = line.slice(3).replace(/^ /, '')
      else if (line.startsWith('event:')) event = line.slice(6).replace(/^ /, '') || 'message'
    }
    if (dataLines.length === 0) return
    const payload = dataLines.join('\n')
    if (!payload || payload === '[DONE]') return
    try {
      onEvent({ id, event, data: JSON.parse(payload) as E })
    } catch {
      // 单条脏数据不应截断后续已持久化事件；调用方仍可用游标重连。
    }
  }

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let separator: number
      while ((separator = buffer.search(/\r?\n\r?\n/)) !== -1) {
        const block = buffer.slice(0, separator)
        buffer = buffer.slice(separator).replace(/^\r?\n\r?\n/, '')
        dispatch(block.replace(/\r/g, ''))
      }
    }
    buffer += decoder.decode()
    if (buffer.trim()) dispatch(buffer.replace(/\r/g, ''))
  } finally {
    reader.releaseLock()
  }
}

/** POST SSE（用于模型流式生成）。 */
export async function ssePost<E>({ url, body, signal, onEvent }: SsePostOptions<E>): Promise<void> {
  const response = await fetch(url, {
    method: 'POST',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  })
  return consumeSSE(response, (frame: SseFrame<E>) => onEvent(frame.data))
}

/** GET SSE（用于 Agent Run 等可按 Last-Event-ID 续传的事件账本）。 */
export async function sseGet<E>({ url, signal, lastEventId, onEvent }: SseGetOptions<E>): Promise<void> {
  const response = await fetch(url, {
    method: 'GET',
    headers: {
      ...authHeaders(),
      ...(lastEventId !== undefined ? { 'Last-Event-ID': String(lastEventId) } : {}),
    },
    signal,
  })
  return consumeSSE(response, onEvent)
}
