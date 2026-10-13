/**
 * IM 会话联系方式识别：电话 / 邮箱 / 微信号，供坐席一键入库。
 */

export type ContactKind = 'phone' | 'email' | 'wechat'

export type ContactHit = {
  kind: ContactKind
  value: string
  /** 展示标签 */
  label: string
}

const KIND_LABEL: Record<ContactKind, string> = {
  phone: '电话',
  email: '邮箱',
  wechat: '微信',
}

export function contactKindLabel(kind: ContactKind): string {
  return KIND_LABEL[kind]
}

/** 去重键：kind + 规范化 value */
export function contactKey(hit: Pick<ContactHit, 'kind' | 'value'>): string {
  return `${hit.kind}:${normalizeContactValue(hit.kind, hit.value)}`
}

export function normalizeContactValue(kind: ContactKind, raw: string): string {
  const s = String(raw || '').trim()
  if (!s) return ''
  if (kind === 'phone') return phoneKey(s)
  if (kind === 'email') return s.toLowerCase()
  // wechat：小写 + 去空白
  return s.replace(/\s+/g, '').toLowerCase()
}

// ---------- 电话 ----------

/** 去掉分隔符后的数字键（用于去重、对比是否已保存） */
export function phoneKey(raw: string): string {
  const s = String(raw || '').trim()
  if (!s) return ''
  const hasPlus = s.startsWith('+')
  const digits = s.replace(/\D/g, '')
  if (!digits) return ''
  // 国内常见 86 前缀统一成 11 位手机号键
  if (digits.length === 13 && digits.startsWith('86') && digits[2] === '1') {
    return digits.slice(2)
  }
  return hasPlus ? `+${digits}` : digits
}

/** 展示用规范化：优先 11 位手机，去掉多余分隔符 */
export function formatPhoneDisplay(raw: string): string {
  const key = phoneKey(raw)
  if (!key) return raw.trim()
  if (key.length === 11 && key.startsWith('1')) return key
  if (key.startsWith('+')) return key
  return key
}

/**
 * 从一段自然语言里提取疑似电话号码。
 * 覆盖：大陆手机、带分隔符手机、+86、区号座机、400/800。
 */
export function extractPhones(text: string): string[] {
  if (!text) return []
  const found: string[] = []
  const seen = new Set<string>()

  const push = (raw: string) => {
    const display = formatPhoneDisplay(raw)
    const key = phoneKey(display)
    if (!key || key.length < 7 || key.length > 15) return
    if (!isPlausiblePhone(key)) return
    if (seen.has(key)) return
    seen.add(key)
    found.push(display)
  }

  const intlMobile = /(?:\+?86|0086)[\s-]*1[3-9]\d[\s-]?\d{4}[\s-]?\d{4}/g
  const cnMobile = /(?<![0-9])1[3-9]\d[\s-]?\d{4}[\s-]?\d{4}(?![0-9])/g
  const landline = /(?<![0-9])0\d{2,3}[\s-]?\d{7,8}(?![0-9])/g
  const service = /(?<![0-9])[48]00[\s-]?\d{3}[\s-]?\d{4}(?![0-9])/g

  for (const re of [intlMobile, cnMobile, landline, service]) {
    re.lastIndex = 0
    let m: RegExpExecArray | null
    while ((m = re.exec(text)) !== null) {
      push(m[0])
    }
  }

  return found
}

function isPlausiblePhone(key: string): boolean {
  const d = key.startsWith('+') ? key.slice(1) : key
  if (/^1[3-9]\d{9}$/.test(d)) return true
  if (/^86[1][3-9]\d{9}$/.test(d)) return true
  if (/^0\d{9,11}$/.test(d)) return true
  if (/^[48]00\d{7}$/.test(d)) return true
  if (key.startsWith('+') && d.length >= 7 && d.length <= 15) return true
  return false
}

// ---------- 邮箱 ----------

/**
 * 提取邮箱。避免匹配到 URL 路径里的伪邮箱。
 */
export function extractEmails(text: string): string[] {
  if (!text) return []
  const found: string[] = []
  const seen = new Set<string>()
  // 常见邮箱；TLD 2～24 字母
  const re =
    /(?<![\w.+-])[a-zA-Z0-9](?:[a-zA-Z0-9._%+-]{0,62}[a-zA-Z0-9])?@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z]{2,24})+(?![\w.-])/g
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    const email = m[0].toLowerCase()
    // 过滤明显噪声
    if (email.length > 128) continue
    if (email.includes('..') || email.startsWith('.') || email.includes('.@')) continue
    if (seen.has(email)) continue
    seen.add(email)
    found.push(email)
  }
  return found
}

// ---------- 微信号 ----------

/** 微信官方账号规则近似：6～20 位，字母开头，可含数字 _ - */
const WECHAT_ID = /[a-zA-Z][-_a-zA-Z0-9]{5,19}/

/**
 * 提取微信号。优先「微信/微信号/wx/wechat/v信」等显式标注；
 * 也识别「微信同号」→ 同条文本中的手机号作为微信。
 */
export function extractWechats(text: string): string[] {
  if (!text) return []
  const found: string[] = []
  const seen = new Set<string>()

  const push = (raw: string) => {
    let v = String(raw || '').trim().replace(/\s+/g, '')
    // 去掉常见包装符号
    v = v.replace(/^[@#]+/, '').replace(/[，,。.;；!！?？]+$/, '')
    if (!v) return
    // 手机号也可作微信号（同号场景）
    const asPhone = formatPhoneDisplay(v)
    if (isPlausiblePhone(phoneKey(asPhone))) {
      const k = phoneKey(asPhone)
      if (!seen.has(k)) {
        seen.add(k)
        found.push(asPhone)
      }
      return
    }
    if (!WECHAT_ID.test(v)) return
    // 重新匹配完整串
    const m = v.match(WECHAT_ID)
    if (!m) return
    const id = m[0]
    // 排除常见假阳性英文词
    if (isWechatFalsePositive(id)) return
    const key = id.toLowerCase()
    if (seen.has(key)) return
    seen.add(key)
    found.push(id)
  }

  // 1) 显式标签：微信号 / 微信 / 微信id / wx / wechat / v信 / VX / WeChat
  const labeled =
    /(?:微信号|微信\s*id|微信ID|微信|v信|V信|vx|VX|wx|WX|wechat|WeChat|WECHAT)\s*[：:=\-–—]?\s*([a-zA-Z][-_a-zA-Z0-9]{5,19}|1[3-9]\d[\s-]?\d{4}[\s-]?\d{4})/gi
  let m: RegExpExecArray | null
  while ((m = labeled.exec(text)) !== null) {
    push(m[1])
  }

  // 2) 「加我微信 xxx」「微信搜 xxx」「微信是 xxx」
  const soft =
    /(?:加我微信|微信搜|微信是|微信为|我的微信(?:号)?(?:是|为)?)\s*[：:=\-–—]?\s*([a-zA-Z][-_a-zA-Z0-9]{5,19}|1[3-9]\d[\s-]?\d{4}[\s-]?\d{4})/gi
  while ((m = soft.exec(text)) !== null) {
    push(m[1])
  }

  // 3) 微信同号 / 微信就是手机号 → 用本句已识别手机号
  if (/(?:微信同号|微信号?同手机|微信就是手机|手机号?即微信|微信和手机一样)/i.test(text)) {
    for (const p of extractPhones(text)) push(p)
  }

  return found
}

function isWechatFalsePositive(id: string): boolean {
  const lower = id.toLowerCase()
  // 纯短英文常见词 / 系统词
  const block = new Set([
    'visitor', 'message', 'system', 'agent', 'please', 'thanks', 'hello',
    'contact', 'number', 'mobile', 'wechat', 'https', 'http',
  ])
  if (block.has(lower)) return true
  // 全数字已在手机号路径处理；这里若是类似 version 编号则过滤
  if (/^[a-z]+\d{0,2}$/i.test(id) && id.length < 8) return true
  return false
}

// ---------- 汇总 ----------

/** 从一段文本提取全部联系方式（电话、邮箱、微信） */
export function extractContacts(text: string): ContactHit[] {
  if (!text) return []
  const hits: ContactHit[] = []
  const seen = new Set<string>()
  const add = (kind: ContactKind, value: string) => {
    const v = kind === 'phone' ? formatPhoneDisplay(value) : value.trim()
    if (!v) return
    const key = contactKey({ kind, value: v })
    if (seen.has(key)) return
    seen.add(key)
    hits.push({ kind, value: v, label: KIND_LABEL[kind] })
  }
  for (const p of extractPhones(text)) add('phone', p)
  for (const e of extractEmails(text)) add('email', e)
  for (const w of extractWechats(text)) add('wechat', w)
  return hits
}

/** 从 IM 消息 content JSON / 纯文本取出可读文本 */
export function messagePlainText(content: string): string {
  if (!content) return ''
  try {
    const o = JSON.parse(content) as { text?: string; event?: string; url?: string; name?: string }
    if (o.event) return ''
    if (typeof o.text === 'string') return o.text
    if (o.url) return o.name || ''
    return content
  } catch {
    return content
  }
}

/** 扫描多条消息，按出现顺序返回去重后的联系方式 */
export function collectContactsFromMessages(
  messages: Array<{ sender_type: string; content: string }>,
  senderFilter: string | string[] = 'visitor',
): ContactHit[] {
  const allow = new Set(Array.isArray(senderFilter) ? senderFilter : [senderFilter])
  const ordered: ContactHit[] = []
  const seen = new Set<string>()
  for (const m of messages) {
    if (!allow.has(m.sender_type)) continue
    for (const hit of extractContacts(messagePlainText(m.content))) {
      const k = contactKey(hit)
      if (seen.has(k)) continue
      seen.add(k)
      ordered.push(hit)
    }
  }
  return ordered
}

/** @deprecated 使用 collectContactsFromMessages */
export function collectPhonesFromMessages(
  messages: Array<{ sender_type: string; content: string }>,
  senderFilter: string | string[] = 'visitor',
): string[] {
  return collectContactsFromMessages(messages, senderFilter)
    .filter((h) => h.kind === 'phone')
    .map((h) => h.value)
}

/** 高亮文本中的联系方式（电话/邮箱/微信） */
export function highlightContactsInText(text: string): Array<string | { kind: ContactKind; value: string }> {
  const contacts = extractContacts(text)
  if (!contacts.length) return [text]

  // 构造可匹配的原文片段列表（按长度降序）
  type Span = { start: number; end: number; kind: ContactKind; value: string }
  const spans: Span[] = []

  const tryFind = (kind: ContactKind, value: string) => {
    // 在原文中找 value 或松散变体
    let idx = text.indexOf(value)
    if (idx < 0 && kind === 'email') {
      idx = text.toLowerCase().indexOf(value.toLowerCase())
    }
    if (idx < 0 && kind === 'phone') {
      // 按数字骨架在原文找
      const digits = value.replace(/\D/g, '')
      if (!digits) return
      const re = new RegExp(digits.split('').join('[\\s-]*'))
      const m = re.exec(text)
      if (m) {
        spans.push({ start: m.index, end: m.index + m[0].length, kind, value: m[0] })
      }
      return
    }
    if (idx < 0 && kind === 'wechat') {
      // 忽略大小写找
      const re = new RegExp(value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i')
      const m = re.exec(text)
      if (m) spans.push({ start: m.index, end: m.index + m[0].length, kind, value: m[0] })
      return
    }
    if (idx >= 0) {
      spans.push({ start: idx, end: idx + value.length, kind, value: text.slice(idx, idx + value.length) })
    }
  }

  for (const c of contacts) tryFind(c.kind, c.value)

  // 去重叠，保留先出现的
  spans.sort((a, b) => a.start - b.start || b.end - a.end - (b.start - a.start))
  const picked: Span[] = []
  let cursor = 0
  for (const s of spans) {
    if (s.start < cursor) continue
    picked.push(s)
    cursor = s.end
  }

  if (!picked.length) return [text]

  const parts: Array<string | { kind: ContactKind; value: string }> = []
  let last = 0
  for (const s of picked) {
    if (s.start > last) parts.push(text.slice(last, s.start))
    parts.push({ kind: s.kind, value: s.value })
    last = s.end
  }
  if (last < text.length) parts.push(text.slice(last))
  return parts
}
