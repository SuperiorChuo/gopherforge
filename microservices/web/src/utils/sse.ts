import { getToken } from '@/utils/request'

/**
 * SSE POST 工具：axios 拦截器不支持流式响应，这里用 fetch + ReadableStream
 * 手写 text/event-stream 解析。Authorization 与 axios 实例同源
 * （@/utils/request 的 getToken，localStorage 'access_token'）。
 *
 * 事件按 SSE 规范以空行分隔，聚合多行 data: 后逐条回调 onEvent。
 * 传入 AbortSignal 可随时停止生成（fetch abort 会让后端感知断开）。
 */
export interface SsePostOptions<E> {
  url: string
  body: unknown
  signal?: AbortSignal
  onEvent: (event: E) => void
}

export async function ssePost<E>({ url, body, signal, onEvent }: SsePostOptions<E>): Promise<void> {
  const token = getToken()
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  })

  if (!response.ok) {
    // 非流式失败（401/500 等）通常是标准 JSON envelope，尽力取出 message
    let msg = `请求失败 (${response.status})`
    try {
      const data = await response.json()
      if (data && typeof data.message === 'string' && data.message) msg = data.message
    } catch {
      // 忽略非 JSON 响应体
    }
    throw new Error(msg)
  }
  if (!response.body) {
    throw new Error('当前环境不支持流式响应')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''

  // 一个 SSE 事件块（以空行结尾）→ 聚合其中的 data: 行并解析 JSON
  const dispatch = (rawBlock: string) => {
    const dataLines: string[] = []
    for (const line of rawBlock.split('\n')) {
      if (line.startsWith('data:')) {
        // 规范允许 "data: xxx" 或 "data:xxx"，去掉一个前导空格
        dataLines.push(line.slice(5).replace(/^ /, ''))
      }
      // 忽略 event:/id:/retry:/注释行——契约只用 data 承载 JSON
    }
    if (dataLines.length === 0) return
    const payload = dataLines.join('\n')
    if (!payload || payload === '[DONE]') return
    try {
      onEvent(JSON.parse(payload) as E)
    } catch {
      // 单条脏数据不中断整个流
    }
  }

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      // 事件之间以空行分隔（兼容 \r\n）
      let sep: number
      while ((sep = buffer.search(/\r?\n\r?\n/)) !== -1) {
        const block = buffer.slice(0, sep)
        buffer = buffer.slice(sep).replace(/^\r?\n\r?\n/, '')
        dispatch(block.replace(/\r/g, ''))
      }
    }
    // 流结束时冲掉残余（无结尾空行的最后一个事件）
    buffer += decoder.decode()
    if (buffer.trim()) dispatch(buffer.replace(/\r/g, ''))
  } finally {
    reader.releaseLock()
  }
}
