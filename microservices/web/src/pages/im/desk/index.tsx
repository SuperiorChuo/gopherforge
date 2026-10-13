import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useSearchParams } from 'react-router-dom'
import {
  Badge,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  Layout,
  List,
  Modal,
  Popconfirm,
  Popover,
  Rate,
  Select,
  Space,
  Segmented,
  Tag,
  Typography,
  Tooltip,
  Upload,
} from 'antd'
import { PaperClipOutlined, ThunderboltOutlined, IdcardOutlined, PlusOutlined, DeleteOutlined, SearchOutlined, FileAddOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import { createLead as createCrmLead, lookupPhone } from '@/api/crm'
import { createTicket } from '@/api/ticket'
import Customer360Panel from '@/components/customer360/Customer360Panel'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill, { type StatusTone } from '@/components/common/StatusPill'
import {
  acceptConversation,
  closeConversation,
  createQuickReply,
  deleteQuickReply,
  getAgentMe,
  getConversationVisitor,
  getVisitorHistory,
  recallMessage,
  searchImMessages,
  setVisitorBan,
  listAgentConversations,
  listMessages,
  listOnlineAgents,
  listQuickReplies,
  listSkillGroups,
  markAgentRead,
  sendAgentAttachment,
  sendAgentMessage,
  setAgentPresence,
  setVisitorContact,
  setVisitorNote,
  summarizeConversation,
  transferConversation,
  uploadImAttachment,
  type ImConversation,
  type ImMessage,
  type ImMessageHit,
  type ImPresence,
  type ImQuickReply,
  type ImSkillGroup,
  type ImVisitorCard,
  type ImVisitorContext,
} from '@/api/im'
import { formatDateTime } from '@/utils/format'
import {
  collectContactsFromMessages,
  contactKey,
  contactKindLabel,
  extractContacts,
  formatPhoneDisplay,
  highlightContactsInText,
  messagePlainText,
  normalizeContactValue,
  type ContactHit,
} from '@/utils/imContact'
import { getToken } from '@/utils/request'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'

const { Sider, Content } = Layout
const { Text } = Typography

function formatBytes(n?: number) {
  if (!n || n <= 0) return ''
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`
  return `${(n / 1024 / 1024).toFixed(1)}MB`
}

function ctxStr(ctx: ImVisitorContext | undefined, ...keys: string[]): string {
  if (!ctx) return ''
  for (const k of keys) {
    const v = ctx[k]
    if (typeof v === 'string' && v.trim()) return v.trim()
  }
  return ''
}

function renderHighlightedContacts(text: string, t: TFunction) {
  const parts = highlightContactsInText(text)
  return (
    <>
      {parts.map((p, i) =>
        typeof p === 'string' ? (
          <span key={i}>{p}</span>
        ) : (
          <span
            key={i}
            className={`im-phone-hl im-contact-hl-${p.kind}`}
            title={t('识别到的{{label}}', { label: contactKindLabel(p.kind) })}
          >
            {p.value}
          </span>
        ),
      )}
    </>
  )
}

type KbHit = { title?: string; snippet?: string; document_id?: number; score?: number }

function renderContent(content: string, highlightContact = false, t: TFunction) {
  try {
    const o = JSON.parse(content)
    if (o.event === 'recalled') return t('（消息已撤回）')
    if (o.event === 'offline_notice') return t('[系统] {{text}}', { text: o.text || t('坐席离线，欢迎留言') })
    if (o.event) return t('[系统] {{text}}', { text: `${o.event}${o.note ? ` · ${o.note}` : ''}` })
    if (o.url) {
      // 安全测试 2026-08-12：附件/图片 URL 仅允许 http(s)/blob/data:image——
      // 防止被污染的 url 注入 javascript: 等协议（点击即执行）。
      const u = String(o.url)
      if (!/^(https?:|blob:|data:image\/)/i.test(u)) {
        return <Text type="secondary">{t('（不支持的链接）')}</Text>
      }
      const isImage = /\.(jpe?g|png|gif|webp)$/i.test(u)
      if (isImage) {
        return (
          <a href={u} target="_blank" rel="noreferrer">
            <img src={u} alt={o.name || t('图片')} style={{ maxWidth: 220, maxHeight: 180, borderRadius: 8, display: 'block' }} />
          </a>
        )
      }
      return (
        <a href={u} target="_blank" rel="noreferrer" download={o.name}>
          📎 {o.name || t('附件')}{o.size ? ` (${formatBytes(o.size)})` : ''}
        </a>
      )
    }
    const text = o.text || content
    const body =
      highlightContact && typeof text === 'string' ? renderHighlightedContacts(text, t) : text
    const hits: KbHit[] = Array.isArray(o.kb_hits) ? o.kb_hits : []
    const kbMode = typeof o.kb_mode === 'string' ? o.kb_mode : ''
    if (hits.length === 0 && kbMode !== 'miss' && kbMode !== 'rag') {
      return body
    }
    return (
      <div className="im-msg-kb">
        <div>{body}</div>
        {hits.length > 0 ? (
          <div className="im-msg-kb-cites">
            <div className="im-msg-kb-cites-title">{t('知识库引用')}</div>
            {hits.map((h, i) => (
              <div key={i} className="im-msg-kb-cite">
                <Tag color="purple" style={{ marginInlineEnd: 6 }}>
                  {h.title || t('文档 #{{n}}', { n: h.document_id || i + 1 })}
                </Tag>
                {h.snippet ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {h.snippet}
                  </Text>
                ) : null}
              </div>
            ))}
          </div>
        ) : kbMode === 'miss' ? (
          <div className="im-msg-kb-miss">
            <Tag>{t('知识库未命中')}</Tag>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {t('已记入「待补充知识」')}
            </Text>
          </div>
        ) : null}
      </div>
    )
  } catch {
    return highlightContact ? renderHighlightedContacts(content, t) : content
  }
}

function statusColor(s: string) {
  if (s === 'queued') return 'orange'
  if (s === 'assigned') return 'blue'
  if (s === 'bot_serving') return 'purple'
  if (s === 'closed') return 'default'
  return 'default'
}

const PRESENCE_PILL: Record<string, { tone: StatusTone; label: string }> = {
  online: { tone: 'success', label: '在线' },
  busy: { tone: 'warning', label: '示忙' },
  offline: { tone: 'muted', label: '离线' },
}

type Scope = 'all' | 'mine' | 'queue' | 'bot' | 'closed'

export default function ImDeskPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const deepLinkDone = useRef(false)
  const [list, setList] = useState<ImConversation[]>([])
  const [active, setActive] = useState<ImConversation | null>(null)
  const [messages, setMessages] = useState<ImMessage[]>([])
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [scope, setScope] = useState<Scope>('all')
  const [presence, setPresence] = useState<string>('offline')
  const [myUserId, setMyUserId] = useState<number>(0)
  const [queueSize, setQueueSize] = useState(0)
  const [skillGroups, setSkillGroups] = useState<ImSkillGroup[]>([])
  const [filterSg, setFilterSg] = useState<number | undefined>()
  const [transferOpen, setTransferOpen] = useState(false)
  const [agents, setAgents] = useState<ImPresence[]>([])
  const [transferTarget, setTransferTarget] = useState<number | undefined>()
  const [transferSg, setTransferSg] = useState<number | undefined>()
  const [transferNote, setTransferNote] = useState('')
  const [closeReason, setCloseReason] = useState('agent')
  // M5：快捷回复面板 + 访客信息卡
  const [quickReplies, setQuickReplies] = useState<ImQuickReply[]>([])
  const [qrOpen, setQrOpen] = useState(false)
  const [qrTitle, setQrTitle] = useState('')
  const [qrContent, setQrContent] = useState('')
  const [visitorOpen, setVisitorOpen] = useState(false)
  const [visitorCard, setVisitorCard] = useState<ImVisitorCard | null>(null)
  const [phoneDraft, setPhoneDraft] = useState('')
  const [emailDraft, setEmailDraft] = useState('')
  const [wechatDraft, setWechatDraft] = useState('')
  const [nameDraft, setNameDraft] = useState('')
  const [contactSaving, setContactSaving] = useState(false)
  const [noteDraft, setNoteDraft] = useState('')
  const [noteSaving, setNoteSaving] = useState(false)
  const [visitorHistory, setVisitorHistory] = useState<ImConversation[]>([])
  // 当前会话已入库联系方式（轻量缓存，供一键保存对比）
  const [savedContacts, setSavedContacts] = useState<{ phone: string; email: string; wechat: string }>({
    phone: '',
    email: '',
    wechat: '',
  })
  const [savingContactKey, setSavingContactKey] = useState<string | null>(null)
  // M7：跨会话消息搜索
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchKw, setSearchKw] = useState('')
  const [searchHits, setSearchHits] = useState<ImMessageHit[]>([])
  const [searching, setSearching] = useState(false)

  /** 访客消息中识别到的联系方式（电话/邮箱/微信） */
  const detectedContacts = useMemo(
    () => collectContactsFromMessages(messages, 'visitor'),
    [messages],
  )
  const unsavedContacts = useMemo(() => {
    return detectedContacts.filter((hit) => {
      const saved = savedContacts[hit.kind] || ''
      if (!saved) return true
      return normalizeContactValue(hit.kind, hit.value) !== normalizeContactValue(hit.kind, saved)
    })
  }, [detectedContacts, savedContacts])

  function applySavedFromVisitor(v: { phone?: string; email?: string; wechat?: string }) {
    setSavedContacts({
      phone: v.phone || '',
      email: v.email || '',
      wechat: v.wechat || '',
    })
    setPhoneDraft(v.phone || '')
    setEmailDraft(v.email || '')
    setWechatDraft(v.wechat || '')
  }

  async function onSearchMessages() {
    const kw = searchKw.trim()
    if (!kw) return
    setSearching(true)
    try {
      const res = await searchImMessages(kw)
      setSearchHits(res.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('搜索失败'))
    } finally {
      setSearching(false)
    }
  }

  // 从搜索/历史跳转到会话（列表里可能没有 closed 会话，直接构造最小对象）
  async function jumpToConversation(publicId: string) {
    setSearchOpen(false)
    setVisitorOpen(false)
    const existing = list.find((c) => c.public_id === publicId)
    const target = existing ?? ({ public_id: publicId } as ImConversation)
    setActive(target)
    await loadMessages(target)
  }
  const [soundOn, setSoundOn] = useState(() => localStorage.getItem('im_desk_sound') !== 'off')

  // 新消息提示音：WebAudio 短促两声，无音频资源依赖
  const playDing = useCallback(() => {
    try {
      const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      const ctx = new Ctx()
      const now = ctx.currentTime
      ;[880, 1174.66].forEach((freq, i) => {
        const osc = ctx.createOscillator()
        const gain = ctx.createGain()
        osc.type = 'sine'
        osc.frequency.value = freq
        gain.gain.setValueAtTime(0.0001, now + i * 0.12)
        gain.gain.exponentialRampToValueAtTime(0.12, now + i * 0.12 + 0.02)
        gain.gain.exponentialRampToValueAtTime(0.0001, now + i * 0.12 + 0.11)
        osc.connect(gain).connect(ctx.destination)
        osc.start(now + i * 0.12)
        osc.stop(now + i * 0.12 + 0.12)
      })
      setTimeout(() => void ctx.close(), 500)
    } catch {
      /* 无声环境忽略 */
    }
  }, [])

  const toggleSound = () => {
    setSoundOn((prev) => {
      localStorage.setItem('im_desk_sound', prev ? 'off' : 'on')
      return !prev
    })
  }

  async function onRecall(m: ImMessage) {
    if (!active) return
    try {
      await recallMessage(active.public_id, m.id)
      setMessages((prev) => prev.map((x) => (x.id === m.id ? { ...x, msg_type: 'event', content: JSON.stringify({ event: 'recalled', by: 'agent' }) } : x)))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('撤回失败'))
    }
  }

  async function onToggleBan() {
    if (!active || !visitorCard) return
    const next = !visitorCard.visitor.banned
    try {
      await setVisitorBan(active.public_id, next)
      setVisitorCard((prev) => (prev ? { ...prev, visitor: { ...prev.visitor, banned: next } } : prev))
      message.success(next ? t('已拉黑该访客') : t('已解除拉黑'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('操作失败'))
    }
  }

  // IM 会话一键转工单（ticket-service M1）
  const [ticketOpen, setTicketOpen] = useState(false)
  const [ticketTitle, setTicketTitle] = useState('')
  const [ticketDesc, setTicketDesc] = useState('')
  const [ticketPriority, setTicketPriority] = useState('normal')
  const [creatingTicket, setCreatingTicket] = useState(false)
  // 客户 360 侧栏刷新令牌（建单 / 保存联系方式后 bump）
  const [panelRefresh, setPanelRefresh] = useState(0)
  const [panelCollapsed, setPanelCollapsed] = useState(false)

  function openCreateTicket() {
    if (!active) return
    const v = visitorCard?.visitor
    const name = v?.display_name || ''
    const phone = (v?.phone || '').trim()
    const defaultTitle = name
      ? t('IM 咨询：{{name}}{{phone}}', { name, phone: phone ? ` (${phone})` : '' })
      : t('IM 会话 {{pid}}', { pid: active.public_id.slice(0, 8) })
    // 描述优先用会话小结，否则摘最近几条访客消息
    let desc = active.summary || ''
    if (!desc) {
      const recent = messages
        .filter((m) => m.sender_type === 'visitor')
        .slice(-6)
        .map((m) => messagePlainText(m.content || ''))
        .filter(Boolean)
      if (recent.length) desc = recent.join('\n')
    }
    setTicketTitle(defaultTitle)
    setTicketDesc(desc)
    setTicketPriority('normal')
    setTicketOpen(true)
    if (!visitorCard) void openVisitorCard()
  }

  async function onCreateTicket() {
    if (!active) return
    const title = ticketTitle.trim()
    if (!title) {
      message.warning(t('请填写工单标题'))
      return
    }
    setCreatingTicket(true)
    try {
      const v = visitorCard?.visitor
      const tk = await createTicket({
        title,
        description: ticketDesc.trim(),
        priority: ticketPriority,
        source: 'im',
        conversation_public_id: active.public_id,
        im_visitor_id: v?.id,
        contact_name: v?.display_name || undefined,
        contact_phone: (v?.phone || '').trim() || undefined,
      })
      message.success(t('已创建工单 {{n}}', { n: tk.ticket_no }))
      setTicketOpen(false)
      setPanelRefresh((n) => n + 1)
      window.open(`/ticket?keyword=${encodeURIComponent(tk.ticket_no)}`, '_blank')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建工单失败'))
    } finally {
      setCreatingTicket(false)
    }
  }

  // IM 访客转入 CRM 销售线索（source=im）：有电话先查重，已是客户/线索则直达不重复建
  const [convertingLead, setConvertingLead] = useState(false)
  async function convertVisitorToLead(card: ImVisitorCard) {
    const v = card.visitor
    const phone = (v.phone || '').trim()
    setConvertingLead(true)
    try {
      if (phone) {
        const hit = await lookupPhone(phone)
        if (hit.matched === 'customer' && hit.customer) {
          message.info(t('该号码已是客户「{{name}}」，为您跳转', { name: hit.customer.name }))
          window.open(`/crm/customers?keyword=${encodeURIComponent(phone)}`, '_blank')
          return
        }
        if (hit.matched === 'lead' && hit.lead) {
          message.info(t('该号码已有线索「{{name}}」，为您跳转', { name: hit.lead.name }))
          window.open(`/crm/leads?keyword=${encodeURIComponent(phone)}`, '_blank')
          return
        }
      }
      const name = v.display_name || t('IM访客 #{{id}}', { id: v.id })
      const notePieces = [`IM 访客 guest_key=${v.guest_key}`]
      if (v.note) notePieces.push(`坐席备注：${v.note}`)
      await createCrmLead({ name, phone, source: 'im', im_visitor_id: v.id, note: notePieces.join('；') })
      message.success(t('已转入销售线索'))
      setPanelRefresh((n) => n + 1)
      window.open(`/crm/leads?keyword=${encodeURIComponent(phone || name)}`, '_blank')
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('转入失败'))
    } finally {
      setConvertingLead(false)
    }
  }

  async function onConvertToLead() {
    if (!visitorCard) return
    await convertVisitorToLead(visitorCard)
  }

  /** 360 侧栏「转线索」：无访客卡时先拉再转 */
  async function onConvertToLeadFromPanel() {
    if (!active) return
    let card = visitorCard
    if (!card) {
      try {
        card = await getConversationVisitor(active.public_id)
        setVisitorCard(card)
        applySavedFromVisitor(card.visitor)
      } catch (e: unknown) {
        message.error(e instanceof Error ? e.message : t('加载访客失败'))
        return
      }
    }
    await convertVisitorToLead(card)
  }

  const refreshQuickReplies = useCallback(async () => {
    try {
      const data = await listQuickReplies()
      setQuickReplies(data.list || [])
    } catch {
      /* 面板打开时再提示 */
    }
  }, [])

  async function onAddQuickReply() {
    if (!qrTitle.trim() || !qrContent.trim()) {
      message.warning(t('标题和内容都要填'))
      return
    }
    try {
      await createQuickReply({ title: qrTitle.trim(), content: qrContent.trim() })
      setQrTitle('')
      setQrContent('')
      await refreshQuickReplies()
      message.success(t('话术已保存'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  async function onDeleteQuickReply(id: number) {
    try {
      await deleteQuickReply(id)
      await refreshQuickReplies()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('删除失败'))
    }
  }

  async function openVisitorCard() {
    if (!active) return
    setVisitorOpen(true)
    setVisitorCard(null)
    setVisitorHistory([])
    try {
      const card = await getConversationVisitor(active.public_id)
      setVisitorCard(card)
      setNoteDraft(card.visitor.note || '')
      setNameDraft(card.visitor.display_name || '')
      applySavedFromVisitor(card.visitor)
      const hist = await getVisitorHistory(active.public_id)
      setVisitorHistory(hist.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加载访客信息失败'))
    }
  }

  async function onSaveNote() {
    if (!active) return
    setNoteSaving(true)
    try {
      await setVisitorNote(active.public_id, noteDraft)
      message.success(t('备注已保存'))
      setVisitorCard((prev) => (prev ? { ...prev, visitor: { ...prev.visitor, note: noteDraft } } : prev))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存备注失败'))
    } finally {
      setNoteSaving(false)
    }
  }

  async function onSaveContact() {
    if (!active) return
    setContactSaving(true)
    try {
      const phone = phoneDraft.trim()
      const email = emailDraft.trim()
      const wechat = wechatDraft.trim()
      const res = await setVisitorContact(active.public_id, {
        display_name: nameDraft.trim() || undefined,
        phone: phone || undefined,
        email: email || undefined,
        wechat: wechat || undefined,
        clear_phone: !phone && !!visitorCard?.visitor.phone,
        clear_email: !email && !!visitorCard?.visitor.email,
        clear_wechat: !wechat && !!visitorCard?.visitor.wechat,
      })
      message.success(t('联系方式已保存'))
      applySavedFromVisitor(res.visitor)
      setVisitorCard((prev) =>
        prev
          ? {
              ...prev,
              visitor: {
                ...prev.visitor,
                ...res.visitor,
                display_name: res.visitor.display_name || prev.visitor.display_name,
              },
            }
          : prev,
      )
      setNameDraft(res.visitor.display_name || '')
      setPanelRefresh((n) => n + 1)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存联系方式失败'))
    } finally {
      setContactSaving(false)
    }
  }

  /** 从消息一键入库识别到的联系方式 */
  async function onQuickSaveContact(hit: ContactHit) {
    if (!active || !hit.value) return
    const key = contactKey(hit)
    setSavingContactKey(key)
    try {
      const body: { phone?: string; email?: string; wechat?: string } = {}
      if (hit.kind === 'phone') body.phone = formatPhoneDisplay(hit.value)
      else if (hit.kind === 'email') body.email = hit.value.trim()
      else body.wechat = hit.value.trim()

      const res = await setVisitorContact(active.public_id, body)
      applySavedFromVisitor(res.visitor)
      setVisitorCard((prev) =>
        prev ? { ...prev, visitor: { ...prev.visitor, ...res.visitor } } : prev,
      )
      message.success(t('已保存{{label}} {{value}}', { label: contactKindLabel(hit.kind), value: hit.value }))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存{{label}}失败', { label: contactKindLabel(hit.kind) }))
    } finally {
      setSavingContactKey(null)
    }
  }

  function isHitSaved(hit: ContactHit): boolean {
    const saved = savedContacts[hit.kind] || ''
    if (!saved) return false
    return normalizeContactValue(hit.kind, hit.value) === normalizeContactValue(hit.kind, saved)
  }

  const refreshMe = useCallback(async () => {
    try {
      const me = await getAgentMe()
      setMyUserId(me.user_id)
      setPresence(me.presence?.status || 'offline')
      setSkillGroups(me.skill_groups_all || [])
    } catch {
      /* agent token may still work for other endpoints */
      try {
        const sg = await listSkillGroups()
        setSkillGroups(sg.list || [])
      } catch {
        /* ignore */
      }
    }
  }, [])

  const refreshList = useCallback(async () => {
    try {
      const data = await listAgentConversations(scope, filterSg)
      setList(data.list || [])
      const queued = (data.list || []).filter((c) => c.status === 'queued').length
      if (scope === 'queue') setQueueSize(queued)
      else {
        // lightweight queue size from all list
        const all = scope === 'all' ? data.list || [] : null
        if (all) setQueueSize(all.filter((c) => c.status === 'queued').length)
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加载会话失败'))
    }
  }, [scope, filterSg, t])

  const loadMessages = useCallback(async (conv: ImConversation) => {
    try {
      const data = await listMessages(conv.public_id)
      setMessages(data.messages || [])
      setActive(data.conversation || conv)
      // 同步已入库联系方式，供识别条对比「是否已保存」
      try {
        const card = await getConversationVisitor(conv.public_id)
        applySavedFromVisitor(card.visitor)
      } catch {
        applySavedFromVisitor({})
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加载消息失败'))
    }
  }, [t])

  // 打开会话 / 有新消息进来时视为已读（服务端游标单调，重复调用无害）
  const markRead = useCallback(async (publicId: string) => {
    try {
      await markAgentRead(publicId)
    } catch {
      /* 已读失败不打扰坐席 */
    }
  }, [])

  useEffect(() => {
    void refreshMe()
    void refreshQuickReplies()
  }, [refreshMe, refreshQuickReplies])

  // 会话列表轮询；后台标签页暂停，回前台立即补拉
  useVisibilityInterval(() => void refreshList(), 5000)

  // 深链：/im/desk?cid=<public_id>（低分会话 / 外部回流）
  useEffect(() => {
    const cid = searchParams.get('cid')?.trim()
    if (!cid || deepLinkDone.current) return
    deepLinkDone.current = true
    void (async () => {
      try {
        await jumpToConversation(cid)
      } catch {
        /* silent */
      } finally {
        const next = new URLSearchParams(searchParams)
        next.delete('cid')
        setSearchParams(next, { replace: true })
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams])

  // 坐席 WS：全页单条连接（原先是两条，且第一条依赖 active/soundOn，
  // 每切一次会话就断开重连）。服务端事实：AgentHub 广播带 conversation.updated /
  // queue.updated / presence.updated / conversation.read / message.recalled；
  // message.new 只发会话级频道，须 conversation.subscribe 订阅。同一条连接允许
  // 多次订阅（断开时统一退订），所以切会话只需补订，不必重建连接。
  // active/soundOn 走 ref，连接与 UI 状态解耦。
  const activeRef = useRef<ImConversation | null>(null)
  activeRef.current = active
  const soundOnRef = useRef(soundOn)
  soundOnRef.current = soundOn
  const wsRef = useRef<WebSocket | null>(null)

  // WS 事件密集时（批量分配 / 状态翻转）把列表刷新合并成 1.5s 一次（尾部执行），
  // 与 5s 轮询叠加也不打爆列表接口
  const refreshListRef = useRef(refreshList)
  refreshListRef.current = refreshList
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const throttledRefreshList = useCallback(() => {
    if (refreshTimerRef.current !== null) return
    refreshTimerRef.current = setTimeout(() => {
      refreshTimerRef.current = null
      void refreshListRef.current()
    }, 1500)
  }, [])
  useEffect(
    () => () => {
      if (refreshTimerRef.current !== null) clearTimeout(refreshTimerRef.current)
    },
    [],
  )

  useEffect(() => {
    if (import.meta.env.VITE_DEMO === '1') return
    const token = getToken()
    if (!token) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/im/ws?token=${encodeURIComponent(token)}`)
    wsRef.current = ws
    ws.onopen = () => {
      const cur = activeRef.current
      if (cur) {
        ws.send(
          JSON.stringify({ type: 'conversation.subscribe', payload: { conversation_public_id: cur.public_id } }),
        )
      }
    }
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        const cur = activeRef.current
        if (
          msg.type === 'conversation.updated' ||
          msg.type === 'queue.updated' ||
          msg.type === 'presence.updated' ||
          msg.type === 'conversation.read'
        ) {
          throttledRefreshList()
        }
        // 当前会话的实时状态（资料变更 / 读回执）
        if (msg.type === 'conversation.updated' && cur && msg.payload?.public_id === cur.public_id) {
          setActive(msg.payload as ImConversation)
        }
        if (msg.type === 'conversation.read' && cur && msg.payload?.conversation_public_id === cur.public_id) {
          const { reader, seq } = msg.payload as { reader: string; seq: number }
          setActive((prev) =>
            prev
              ? reader === 'visitor'
                ? { ...prev, visitor_last_read_seq: seq }
                : { ...prev, agent_last_read_seq: seq }
              : prev,
          )
        }
        if (msg.type === 'message.new') {
          const m = msg.payload?.message as ImMessage
          const convPid = msg.payload?.conversation_public_id as string | undefined
          if (cur && convPid === cur.public_id) {
            setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]))
            if (m.sender_type !== 'agent') {
              void markRead(cur.public_id)
              if (soundOnRef.current) playDing()
            }
          } else if (m?.sender_type === 'visitor') {
            // 非当前会话的新消息也提醒（此前浏览过的会话仍在订阅内，列表未读徽标 + 声音）
            if (soundOnRef.current) playDing()
          }
        }
        if (msg.type === 'message.recalled' && cur && msg.payload?.conversation_public_id === cur.public_id) {
          const m = msg.payload.message as ImMessage
          setMessages((prev) => prev.map((x) => (x.id === m.id ? m : x)))
        }
      } catch {
        /* ignore */
      }
    }
    return () => {
      wsRef.current = null
      ws.close()
    }
  }, [markRead, playDing, throttledRefreshList])

  // 切换会话：在同一条连接上补订阅新会话（服务端同连接可多次订阅），不再新建 WS
  useEffect(() => {
    const pid = active?.public_id
    const ws = wsRef.current
    if (!pid || !ws || ws.readyState !== WebSocket.OPEN) return
    ws.send(JSON.stringify({ type: 'conversation.subscribe', payload: { conversation_public_id: pid } }))
  }, [active?.public_id])

  const title = useMemo(() => {
    if (!active) return t('选择左侧会话')
    const sg = skillGroups.find((g) => g.id === active.skill_group_id)
    return `${t('会话 {{pid}}…', { pid: active.public_id.slice(0, 8) })}${sg ? ` · ${sg.name}` : ''}`
  }, [active, skillGroups, t])

  async function onSelect(item: ImConversation) {
    setActive(item)
    await loadMessages(item)
    await markRead(item.public_id)
    void refreshList()
  }

  async function onPresenceChange(status: string) {
    try {
      const p = await setAgentPresence(status)
      setPresence(p.status)
      message.success(t('状态：{{status}}', { status: p.status }))
      void refreshList()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('更新状态失败'))
    }
  }

  async function onAccept() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await acceptConversation(active.public_id)
      setActive(conv)
      message.success(t('已接入'))
      await refreshList()
      await loadMessages(conv)
    } finally {
      setLoading(false)
    }
  }

  async function onClose() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await closeConversation(active.public_id, closeReason)
      setActive(conv)
      message.success(t('已结束'))
      await refreshList()
      await loadMessages(conv)
    } finally {
      setLoading(false)
    }
  }

  async function openTransfer() {
    try {
      const data = await listOnlineAgents()
      setAgents(data.list || [])
    } catch {
      setAgents([])
    }
    setTransferTarget(undefined)
    setTransferSg(active?.skill_group_id)
    setTransferNote('')
    setTransferOpen(true)
  }

  async function onTransfer() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await transferConversation(active.public_id, {
        target_agent_user_id: transferTarget || 0,
        skill_group_id: transferSg,
        note: transferNote,
      })
      setActive(conv)
      setTransferOpen(false)
      message.success(transferTarget ? t('已转接坐席') : t('已退回排队'))
      await refreshList()
      await loadMessages(conv)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('转接失败'))
    } finally {
      setLoading(false)
    }
  }

  async function onSummary() {
    if (!active) return
    setLoading(true)
    try {
      const data = await summarizeConversation(active.public_id)
      setActive(data.conversation)
      message.success(t('小结已生成'))
      await loadMessages(data.conversation)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('小结失败'))
    } finally {
      setLoading(false)
    }
  }

  async function onSend() {
    if (!active || !text.trim()) return
    const body = text.trim()
    setText('')
    const clientMsgId = crypto.randomUUID()
    try {
      const m = await sendAgentMessage(active.public_id, body, clientMsgId)
      setMessages((prev) => [...prev, m])
      if (active.status === 'queued') {
        await refreshList()
        await loadMessages(active)
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('发送失败'))
    }
  }

  async function onPickFile(file: File) {
    if (!active) return false
    try {
      const att = await uploadImAttachment(file)
      const m = await sendAgentAttachment(active.public_id, att, crypto.randomUUID())
      setMessages((prev) => [...prev, m])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('上传失败'))
    }
    return false
  }

  const sgName = (id?: number) => skillGroups.find((g) => g.id === id)?.name || (id ? `#${id}` : '—')

  return (
    <Card
      className="im-desk-card"
      title={t('智能客服 · 坐席工作台')}
      extra={
        <Space wrap>
          <Text type="secondary">{t('我的状态')}</Text>
          <Segmented
            value={presence}
            onChange={(v) => void onPresenceChange(String(v))}
            options={[
              { label: t('在线'), value: 'online' },
              { label: t('示忙'), value: 'busy' },
              { label: t('离线'), value: 'offline' },
            ]}
          />
          <StatusPill
            tone={(PRESENCE_PILL[presence] ?? PRESENCE_PILL.offline).tone}
            label={(PRESENCE_PILL[presence] ?? PRESENCE_PILL.offline).label}
          />
          <Tag variant="filled" color={queueSize > 0 ? 'orange' : 'default'}>{t('排队')} {queueSize}</Tag>
          <Tooltip title={soundOn ? t('关闭新消息提示音') : t('开启新消息提示音')}>
            <Button onClick={toggleSound}>{soundOn ? '🔔' : '🔕'}</Button>
          </Tooltip>
          <Button href="/im/stats">{t('统计')}</Button>
          <Button href="/im/skills">{t('技能组')}</Button>
          <Button href="/im/sites">{t('站点')}</Button>
        </Space>
      }
      styles={{ body: { padding: 0 } }}
    >
      <Layout style={{ minHeight: 560, background: 'transparent' }}>
        {/* 窄屏（<992px）自动折叠收起，避免 320px 侧栏把内容区挤到视口外 */}
        <Sider width={320} className="im-desk-sider" collapsible breakpoint="lg" collapsedWidth={0}>
          <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Segmented
              block
              value={scope}
              onChange={(v) => setScope(v as Scope)}
              options={[
                { label: t('全部'), value: 'all' },
                { label: t('我的'), value: 'mine' },
                { label: t('排队'), value: 'queue' },
                { label: t('机器人'), value: 'bot' },
                { label: t('已结束'), value: 'closed' },
              ]}
            />
            <Select
              allowClear
              placeholder={t('按技能组筛选')}
              style={{ width: '100%' }}
              value={filterSg}
              onChange={(v) => setFilterSg(v)}
              options={skillGroups.map((g) => ({ label: `${g.name} (${g.code})`, value: g.id }))}
            />
            <Button block onClick={() => void refreshList()}>
              {t('刷新会话')}
            </Button>
          </div>
          <List
            dataSource={list}
            locale={{ emptyText: <GlassEmpty text="暂无会话" compact /> }}
            renderItem={(item) => (
              <List.Item
                className={`im-conv-li${active?.public_id === item.public_id ? ' is-active' : ''}`}
                onClick={() => void onSelect(item)}
              >
                <List.Item.Meta
                  title={
                    <Space wrap size={4}>
                      <span className="cell-mono im-conv-id">{item.public_id.slice(0, 8)}</span>
                      <Tag variant="filled" color={statusColor(item.status)}>{item.status}</Tag>
                      {item.skill_group_id ? (
                        <Tag variant="filled">{sgName(item.skill_group_id)}</Tag>
                      ) : null}
                      {item.agent_user_id && item.agent_user_id === myUserId ? (
                        <Tag variant="filled" color="purple">{t('我')}</Tag>
                      ) : null}
                      {item.rating_score ? (
                        <Tag variant="filled" color="gold">★ {item.rating_score}</Tag>
                      ) : null}
                      <Badge count={item.unread_count || 0} size="small" />
                    </Space>
                  }
                  description={item.last_message_preview || t('（尚无消息）')}
                />
              </List.Item>
            )}
          />
        </Sider>
        <Content style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="im-desk-head">
            <Space size={8}>
              <Text strong>{title}</Text>
              {active?.rating_score ? (
                <Tooltip title={active.rating_comment || '访客评价'}>
                  <Tag variant="filled" color="gold">★ {active.rating_score}</Tag>
                </Tooltip>
              ) : null}
            </Space>
            <Space wrap>
              <Tooltip title={t('搜索全部会话消息')}>
                <Button icon={<SearchOutlined />} onClick={() => setSearchOpen(true)}>{t('搜索')}</Button>
              </Tooltip>
              <Tooltip title={t('访客信息与备注')}>
                <Button icon={<IdcardOutlined />} disabled={!active} onClick={() => void openVisitorCard()}>
                  {t('访客')}
                </Button>
              </Tooltip>
              <Tooltip title={panelCollapsed ? t('展开客户 360') : t('收起客户 360')}>
                <Button disabled={!active && panelCollapsed} onClick={() => setPanelCollapsed((c) => !c)}>
                  {panelCollapsed ? t('客户360') : t('收起360')}
                </Button>
              </Tooltip>
              <Tooltip title={t('从队列手动接入')}>
                <Button
                  disabled={!active || active.status === 'assigned' || active.status === 'closed'}
                  loading={loading}
                  onClick={() => void onAccept()}
                >
                  {t('接入')}
                </Button>
              </Tooltip>
              <Button disabled={!active || active.status === 'closed'} loading={loading} onClick={() => void openTransfer()}>
                {t('转接')}
              </Button>
              <Tooltip title={t('调用 AI/本地机器人生成会话小结')}>
                <Button disabled={!active} loading={loading} onClick={() => void onSummary()}>
                  {t('小结')}
                </Button>
              </Tooltip>
              <Tooltip title={t('将会话转为工单跟进（ticket）')}>
                <Button
                  icon={<FileAddOutlined />}
                  disabled={!active}
                  onClick={() => openCreateTicket()}
                >
                  {t('转工单')}
                </Button>
              </Tooltip>
              <Select
                size="small"
                style={{ width: 120 }}
                value={closeReason}
                onChange={setCloseReason}
                options={[
                  { label: t('坐席结束'), value: 'agent' },
                  { label: t('访客结束'), value: 'visitor' },
                  { label: t('超时'), value: 'timeout' },
                  { label: t('系统'), value: 'system' },
                ]}
              />
              <Button disabled={!active || active.status === 'closed'} loading={loading} danger onClick={() => void onClose()}>
                {t('结束')}
              </Button>
            </Space>
          </div>
          <div className="im-desk-msgs">
            {!active && <GlassEmpty text="从左侧选择会话，或打开访客页发起咨询" />}
            {active?.summary ? (
              <div className="im-desk-summary">
                <Text strong type="secondary">
                  {t('会话小结')} ·{' '}
                </Text>
                {active.summary}
              </div>
            ) : null}
            {active && unsavedContacts.length > 0 ? (
              <div className="im-desk-contact-bar">
                <Text>{t('识别到联系方式')}</Text>
                {unsavedContacts.map((hit) => {
                  const k = contactKey(hit)
                  return (
                    <Space key={k} size={4}>
                      <Tag variant="filled" color={hit.kind === 'phone' ? 'green' : hit.kind === 'email' ? 'blue' : 'cyan'}>
                        {hit.label}
                      </Tag>
                      <span className="im-contact-phone">{hit.value}</span>
                      <Button
                        type="primary"
                        size="small"
                        loading={savingContactKey === k}
                        disabled={!!savingContactKey && savingContactKey !== k}
                        onClick={() => void onQuickSaveContact(hit)}
                      >
                        {t('一键保存')}
                      </Button>
                    </Space>
                  )
                })}
              </div>
            ) : null}
            {active && detectedContacts.length > 0 && unsavedContacts.length === 0 ? (
              <div className="im-desk-contact-bar" style={{ opacity: 0.85 }}>
                <Text type="secondary">{t('联系方式已入库')}</Text>
                {detectedContacts.map((hit) => (
                  <Space key={contactKey(hit)} size={4}>
                    <Tag variant="filled">{hit.label}</Tag>
                    <span className="im-contact-phone">{savedContacts[hit.kind] || hit.value}</span>
                  </Space>
                ))}
              </div>
            ) : null}
            {messages.map((m) => {
              const msgHits =
                m.sender_type === 'visitor' ? extractContacts(messagePlainText(m.content)) : []
              const msgUnsaved = msgHits.filter((h) => !isHitSaved(h))
              return (
                <div key={m.id} className={`im-msg im-msg-${m.sender_type}`}>
                  <div className="im-msg-meta">
                    {m.sender_type} · #{m.seq}
                    {m.sender_type === 'agent' && m.msg_type !== 'event' && active?.status !== 'closed' ? (
                      <Popconfirm title={t('撤回这条消息？（2 分钟内）')} onConfirm={() => void onRecall(m)}>
                        <Button type="link" size="small" style={{ padding: '0 4px', fontSize: 11 }}>{t('撤回')}</Button>
                      </Popconfirm>
                    ) : null}
                  </div>
                  <div>{renderContent(m.content, m.sender_type === 'visitor', t)}</div>
                  {msgUnsaved.length > 0 ? (
                    <div className="im-msg-contact-actions">
                      {msgUnsaved.map((hit) => {
                        const k = contactKey(hit)
                        return (
                          <Button
                            key={k}
                            type="primary"
                            size="small"
                            ghost
                            loading={savingContactKey === k}
                            onClick={() => void onQuickSaveContact(hit)}
                          >
                            {t('保存{{label}} {{value}}', { label: hit.label, value: hit.value })}
                          </Button>
                        )
                      })}
                    </div>
                  ) : null}
                  {m.sender_type === 'agent' ? (
                    <div
                      className={`im-msg-read${(active?.visitor_last_read_seq ?? 0) >= m.seq ? ' is-read' : ''}`}
                    >
                      {(active?.visitor_last_read_seq ?? 0) >= m.seq ? t('已读') : t('未读')}
                    </div>
                  ) : null}
                </div>
              )
            })}
          </div>
          <div className="im-desk-input">
            <Popover
              open={qrOpen}
              onOpenChange={setQrOpen}
              trigger="click"
              placement="topLeft"
              content={
                <div style={{ width: 340 }}>
                  <List
                    size="small"
                    dataSource={quickReplies}
                    locale={{ emptyText: <GlassEmpty text="还没有话术，先在下方添加" compact /> }}
                    style={{ maxHeight: 260, overflow: 'auto' }}
                    renderItem={(qr) => (
                      <List.Item
                        style={{ cursor: 'pointer' }}
                        onClick={() => {
                          setText((prev) => (prev ? `${prev}${qr.content}` : qr.content))
                          setQrOpen(false)
                        }}
                        actions={[
                          <Popconfirm
                            key="del"
                            title={t('删除这条话术？')}
                            onConfirm={(e) => {
                              e?.stopPropagation()
                              void onDeleteQuickReply(qr.id)
                            }}
                            onCancel={(e) => e?.stopPropagation()}
                          >
                            <Button
                              type="text"
                              size="small"
                              danger
                              icon={<DeleteOutlined />}
                              onClick={(e) => e.stopPropagation()}
                            />
                          </Popconfirm>,
                        ]}
                      >
                        <List.Item.Meta
                          title={qr.title}
                          description={<span style={{ fontSize: 12 }}>{qr.content.slice(0, 60)}</span>}
                        />
                      </List.Item>
                    )}
                  />
                  <Space.Compact block style={{ marginTop: 8 }}>
                    <Input placeholder={t('标题')} style={{ width: 96 }} value={qrTitle} onChange={(e) => setQrTitle(e.target.value)} />
                    <Input placeholder={t('话术内容')} value={qrContent} onChange={(e) => setQrContent(e.target.value)} onPressEnter={() => void onAddQuickReply()} />
                    <Button icon={<PlusOutlined />} onClick={() => void onAddQuickReply()} />
                  </Space.Compact>
                </div>
              }
            >
              <Button
                icon={<ThunderboltOutlined />}
                disabled={!active || active.status === 'closed'}
                title={t('快捷回复（团队话术库）')}
              />
            </Popover>
            <Upload
              showUploadList={false}
              beforeUpload={(f) => onPickFile(f as unknown as File)}
              accept=".jpg,.jpeg,.png,.gif,.webp,.pdf,.txt,.zip,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.csv,.mp4,.mp3"
              disabled={!active || active.status === 'closed'}
            >
              <Button icon={<PaperClipOutlined />} disabled={!active || active.status === 'closed'} title={t('发送图片/文件（≤10MB）')} />
            </Upload>
            <Input
              value={text}
              disabled={!active || active.status === 'closed'}
              placeholder={t('输入回复…')}
              onChange={(e) => setText(e.target.value)}
              onPressEnter={() => void onSend()}
            />
            <Button type="primary" disabled={!active || active.status === 'closed'} onClick={() => void onSend()}>
              {t('发送')}
            </Button>
          </div>
          <div className="im-desk-hint">
            {t('提示：机器人接待中可点「小结」；访客「转人工」后进入排队。右侧客户 360 聚合 CRM / 工单 / 历史 / 通话。')} ·{' '}
            <a href="/im/visitor" target="_blank" rel="noreferrer">
              {t('访客 H5')}
            </a>
            {' · '}
            <a href="/im/widget/demo.html" target="_blank" rel="noreferrer">
              {t('埋码演示')}
            </a>
          </div>
        </Content>
        {!panelCollapsed ? (
          <Sider width={300} className="im-desk-360-sider">
            <Customer360Panel
              conversation={active}
              refreshToken={panelRefresh}
              onJumpConversation={(pid) => void jumpToConversation(pid)}
              onCreateTicket={() => openCreateTicket()}
              onConvertLead={() => void onConvertToLeadFromPanel()}
              onEditVisitor={() => void openVisitorCard()}
            />
          </Sider>
        ) : null}
      </Layout>

      <Modal
        title={t('转接会话')}
        open={transferOpen}
        onCancel={() => setTransferOpen(false)}
        onOk={() => void onTransfer()}
        confirmLoading={loading}
        okText={transferTarget ? t('转给坐席') : t('退回排队')}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <div>
            <Text type="secondary">{t('目标坐席（不选则退回排队）')}</Text>
            <Select
              allowClear
              style={{ width: '100%', marginTop: 4 }}
              placeholder={t('选择在线/示忙坐席')}
              value={transferTarget}
              onChange={setTransferTarget}
              options={agents
                .filter((a) => a.agent_user_id !== myUserId)
                .map((a) => ({
                  label: t('{{a}} · {{b}} · 负载 {{n}}', { a: a.display_name || a.agent_user_id, b: a.status, n: a.assigned_count ?? 0 }),
                  value: a.agent_user_id,
                }))}
            />
          </div>
          <div>
            <Text type="secondary">{t('技能组（可选，退回排队时生效）')}</Text>
            <Select
              allowClear
              style={{ width: '100%', marginTop: 4 }}
              placeholder={t('保持原技能组')}
              value={transferSg}
              onChange={setTransferSg}
              options={skillGroups.map((g) => ({ label: `${g.name} (${g.strategy})`, value: g.id }))}
            />
          </div>
          <div>
            <Text type="secondary">{t('备注')}</Text>
            <Input.TextArea
              style={{ marginTop: 4 }}
              rows={2}
              value={transferNote}
              onChange={(e) => setTransferNote(e.target.value)}
              placeholder={t('转接说明，将记入系统事件')}
            />
          </div>
        </Space>
      </Modal>

      <Drawer
        title={t('访客信息')}
        open={visitorOpen}
        onClose={() => setVisitorOpen(false)}
        width={380}
        destroyOnHidden
      >
        {visitorCard ? (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label={t('访客')}>
                {visitorCard.visitor.display_name || t('访客 #{{id}}', { id: visitorCard.visitor.id })}
              </Descriptions.Item>
              <Descriptions.Item label={t('联系电话')}>
                {visitorCard.visitor.phone ? (
                  <a href={`tel:${visitorCard.visitor.phone}`}>{visitorCard.visitor.phone}</a>
                ) : (
                  <Text type="secondary">{t('未留下')}</Text>
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('邮箱')}>
                {visitorCard.visitor.email ? (
                  <a href={`mailto:${visitorCard.visitor.email}`}>{visitorCard.visitor.email}</a>
                ) : (
                  <Text type="secondary">{t('未留下')}</Text>
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('微信')}>
                {visitorCard.visitor.wechat || <Text type="secondary">{t('未留下')}</Text>}
              </Descriptions.Item>
              <Descriptions.Item label="Guest Key">
                <span className="cell-mono" style={{ fontSize: 12 }}>{visitorCard.visitor.guest_key}</span>
              </Descriptions.Item>
              {visitorCard.site_name ? (
                <Descriptions.Item label={t('埋码站点')}>{visitorCard.site_name}</Descriptions.Item>
              ) : null}
              <Descriptions.Item label={t('渠道')}>
                {visitorCard.channel || active?.channel || '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('历史会话')}>{t('{{n}} 次', { n: visitorCard.conversation_count })}</Descriptions.Item>
              <Descriptions.Item label={t('首次访问')}>{formatDateTime(visitorCard.visitor.created_at)}</Descriptions.Item>
              <Descriptions.Item label={t('最近活跃')}>{formatDateTime(visitorCard.visitor.last_seen_at)}</Descriptions.Item>
              {active?.rating_score ? (
                <Descriptions.Item label={t('本次评价')}>
                  <Space direction="vertical" size={4}>
                    <Rate disabled value={active.rating_score} />
                    {active.rating_comment ? <Text type="secondary">{active.rating_comment}</Text> : null}
                  </Space>
                </Descriptions.Item>
              ) : null}
            </Descriptions>

            {(() => {
              const ctx = visitorCard.context || {}
              const pageUrl = ctxStr(ctx, 'page_url', 'page')
              const pageTitle = ctxStr(ctx, 'page_title')
              const referrer = ctxStr(ctx, 'referrer')
              const origin = ctxStr(ctx, 'parent_origin')
              const ip = ctxStr(ctx, 'client_ip')
              const ipLoc = ctxStr(ctx, 'ip_location')
              const hasSource = !!(pageUrl || pageTitle || referrer || origin || ip)
              if (!hasSource) {
                return (
                  <Card size="small" title={t('来源 / 地址')}>
                    <Text type="secondary">{t('暂无页面来源或 IP 信息（旧会话或未走埋码）')}</Text>
                  </Card>
                )
              }
              return (
                <Card size="small" title={t('来源 / 地址')}>
                  <Descriptions column={1} size="small">
                    {pageTitle ? <Descriptions.Item label={t('页面标题')}>{pageTitle}</Descriptions.Item> : null}
                    {pageUrl ? (
                      <Descriptions.Item label={t('来源页面')}>
                        <a href={pageUrl} target="_blank" rel="noreferrer" style={{ wordBreak: 'break-all' }}>
                          {pageUrl}
                        </a>
                      </Descriptions.Item>
                    ) : null}
                    {referrer ? (
                      <Descriptions.Item label={t('上游来源')}>
                        <span style={{ wordBreak: 'break-all' }}>{referrer}</span>
                      </Descriptions.Item>
                    ) : null}
                    {origin ? <Descriptions.Item label={t('站点 Origin')}>{origin}</Descriptions.Item> : null}
                    {ip ? (
                      <Descriptions.Item label={t('访客 IP')}>
                        <span className="cell-mono">{ip}</span>
                        {ipLoc ? <Tag style={{ marginLeft: 8 }} variant="filled">{ipLoc}</Tag> : null}
                      </Descriptions.Item>
                    ) : null}
                  </Descriptions>
                </Card>
              )
            })()}

            <div>
              <Text type="secondary">{t('联系方式（对话可智能识别一键保存，也可手动补录）')}</Text>
              <Input
                style={{ marginTop: 6 }}
                value={nameDraft}
                onChange={(e) => setNameDraft(e.target.value)}
                placeholder={t('称呼')}
                maxLength={64}
              />
              <Input
                style={{ marginTop: 8 }}
                value={phoneDraft}
                onChange={(e) => setPhoneDraft(e.target.value)}
                placeholder={t('手机 / 座机')}
                maxLength={20}
              />
              <Input
                style={{ marginTop: 8 }}
                value={emailDraft}
                onChange={(e) => setEmailDraft(e.target.value)}
                placeholder={t('邮箱')}
                maxLength={128}
              />
              <Input
                style={{ marginTop: 8 }}
                value={wechatDraft}
                onChange={(e) => setWechatDraft(e.target.value)}
                placeholder={t('微信号')}
                maxLength={64}
              />
              <Button type="primary" style={{ marginTop: 8 }} loading={contactSaving} onClick={() => void onSaveContact()}>
                {t('保存联系方式')}
              </Button>
            </div>

            <Space wrap>
              <Tooltip title={visitorCard.visitor.phone ? t('按电话查重后转入 CRM 线索') : t('访客暂无电话，将直接以昵称建线索')}>
                <Button type="primary" ghost loading={convertingLead} onClick={() => void onConvertToLead()}>
                  {t('转入销售线索')}
                </Button>
              </Tooltip>
              <Button icon={<FileAddOutlined />} onClick={() => openCreateTicket()}>
                {t('转工单')}
              </Button>
              <Popconfirm
                title={visitorCard.visitor.banned ? t('解除拉黑该访客？') : t('拉黑后该访客无法发起会话和发言，确认？')}
                onConfirm={() => void onToggleBan()}
              >
                <Button danger={!visitorCard.visitor.banned}>
                  {visitorCard.visitor.banned ? t('解除拉黑') : t('拉黑访客')}
                </Button>
              </Popconfirm>
              {visitorCard.visitor.banned ? <Tag color="red" variant="filled">{t('已拉黑')}</Tag> : null}
            </Space>
            <div>
              <Text type="secondary">{t('坐席备注（团队可见）')}</Text>
              <Input.TextArea
                rows={4}
                style={{ marginTop: 6 }}
                value={noteDraft}
                onChange={(e) => setNoteDraft(e.target.value)}
                placeholder={t('如：VIP 客户 / 已答应回电 / 投诉倾向…')}
                maxLength={2000}
              />
              <Button type="primary" style={{ marginTop: 8 }} loading={noteSaving} onClick={() => void onSaveNote()}>
                {t('保存备注')}
              </Button>
            </div>
            <div>
              <Text type="secondary">{t('历史会话（近 {{n}} 次）', { n: visitorHistory.length })}</Text>
              <List
                size="small"
                style={{ marginTop: 6, maxHeight: 220, overflow: 'auto' }}
                dataSource={visitorHistory}
                locale={{ emptyText: <GlassEmpty text="暂无历史会话" compact /> }}
                renderItem={(h) => (
                  <List.Item
                    style={{ cursor: 'pointer', padding: '6px 4px' }}
                    onClick={() => void jumpToConversation(h.public_id)}
                  >
                    <Space size={6} wrap>
                      <span className="cell-mono" style={{ fontSize: 12 }}>{h.public_id.slice(0, 8)}</span>
                      <Tag variant="filled" color={statusColor(h.status)}>{h.status}</Tag>
                      {h.rating_score ? <Tag variant="filled" color="gold">★ {h.rating_score}</Tag> : null}
                      <Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(h.created_at)}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          </Space>
        ) : (
          <GlassEmpty text="加载中…" compact />
        )}
      </Drawer>

      <Modal
        title={t('搜索会话消息')}
        open={searchOpen}
        onCancel={() => setSearchOpen(false)}
        footer={null}
        width={640}
      >
        <Space.Compact block style={{ marginBottom: 12 }}>
          <Input
            placeholder={t('输入关键词，回车搜索（全部会话的文本消息）')}
            value={searchKw}
            onChange={(e) => setSearchKw(e.target.value)}
            onPressEnter={() => void onSearchMessages()}
            autoFocus
          />
          <Button type="primary" icon={<SearchOutlined />} loading={searching} onClick={() => void onSearchMessages()}>
            {t('搜索')}
          </Button>
        </Space.Compact>
        <List
          size="small"
          style={{ maxHeight: 380, overflow: 'auto' }}
          dataSource={searchHits}
          locale={{ emptyText: <GlassEmpty text={searchKw ? '无匹配消息' : '输入关键词开始搜索'} compact /> }}
          renderItem={(hit) => (
            <List.Item
              style={{ cursor: 'pointer' }}
              onClick={() => void jumpToConversation(hit.conversation_public_id)}
            >
              <List.Item.Meta
                title={
                  <Space size={6}>
                    <span className="cell-mono" style={{ fontSize: 12 }}>{hit.conversation_public_id.slice(0, 8)}</span>
                    <Tag variant="filled" color={statusColor(hit.conversation_status)}>{hit.conversation_status}</Tag>
                    <Tag variant="filled">{hit.sender_type}</Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(hit.created_at)}</Text>
                  </Space>
                }
                description={<span style={{ fontSize: 13 }}>{renderContent(hit.content, false, t)}</span>}
              />
            </List.Item>
          )}
        />
      </Modal>

      <Modal
        title={t('将会话转为工单')}
        open={ticketOpen}
        onCancel={() => setTicketOpen(false)}
        onOk={() => void onCreateTicket()}
        confirmLoading={creatingTicket}
        okText={t('创建工单')}
        destroyOnHidden
        width={520}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <div>
            <Text type="secondary">{t('标题')}</Text>
            <Input
              value={ticketTitle}
              onChange={(e) => setTicketTitle(e.target.value)}
              maxLength={200}
              style={{ marginTop: 4 }}
            />
          </div>
          <div>
            <Text type="secondary">{t('描述（默认带会话小结 / 最近访客消息）')}</Text>
            <Input.TextArea
              rows={5}
              value={ticketDesc}
              onChange={(e) => setTicketDesc(e.target.value)}
              style={{ marginTop: 4 }}
            />
          </div>
          <div>
            <Text type="secondary">{t('优先级')}</Text>
            <Select
              style={{ width: 160, marginTop: 4, display: 'block' }}
              value={ticketPriority}
              onChange={setTicketPriority}
              options={[
                { value: 'low', label: t('低') },
                { value: 'normal', label: t('普通') },
                { value: 'high', label: t('高') },
                { value: 'urgent', label: t('紧急') },
              ]}
            />
          </div>
          {active ? (
            <Text type="secondary" style={{ fontSize: 12 }}>
              {t('将关联会话 {{pid}}', { pid: active.public_id })}
              {visitorCard?.visitor?.id ? t(' · 访客 #{{id}}', { id: visitorCard.visitor.id }) : ''}
            </Text>
          ) : null}
        </Space>
      </Modal>
    </Card>
  )
}
