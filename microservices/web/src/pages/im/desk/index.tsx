import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Input,
  Layout,
  List,
  Modal,
  Select,
  Space,
  Segmented,
  Tag,
  Typography,
  Tooltip,
  Upload,
} from 'antd'
import { PaperClipOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill, { type StatusTone } from '@/components/StatusPill'
import {
  acceptConversation,
  closeConversation,
  getAgentMe,
  listAgentConversations,
  listMessages,
  listOnlineAgents,
  listSkillGroups,
  markAgentRead,
  sendAgentAttachment,
  sendAgentMessage,
  setAgentPresence,
  summarizeConversation,
  transferConversation,
  uploadImAttachment,
  type ImConversation,
  type ImMessage,
  type ImPresence,
  type ImSkillGroup,
} from '@/api/im'
import { getToken } from '@/utils/request'

const { Sider, Content } = Layout
const { Text } = Typography

function formatBytes(n?: number) {
  if (!n || n <= 0) return ''
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`
  return `${(n / 1024 / 1024).toFixed(1)}MB`
}

function renderContent(content: string) {
  try {
    const o = JSON.parse(content)
    if (o.event) return `[系统] ${o.event}${o.note ? ` · ${o.note}` : ''}`
    if (o.url) {
      const isImage = /\.(jpe?g|png|gif|webp)$/i.test(o.url)
      if (isImage) {
        return (
          <a href={o.url} target="_blank" rel="noreferrer">
            <img src={o.url} alt={o.name || '图片'} style={{ maxWidth: 220, maxHeight: 180, borderRadius: 8, display: 'block' }} />
          </a>
        )
      }
      return (
        <a href={o.url} target="_blank" rel="noreferrer" download={o.name}>
          📎 {o.name || '附件'}{o.size ? ` (${formatBytes(o.size)})` : ''}
        </a>
      )
    }
    return o.text || content
  } catch {
    return content
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

type Scope = 'all' | 'mine' | 'queue' | 'bot'

export default function ImDeskPage() {
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
      message.error(e instanceof Error ? e.message : '加载会话失败')
    }
  }, [scope, filterSg])

  const loadMessages = useCallback(async (conv: ImConversation) => {
    try {
      const data = await listMessages(conv.public_id)
      setMessages(data.messages || [])
      setActive(data.conversation || conv)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载消息失败')
    }
  }, [])

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
  }, [refreshMe])

  useEffect(() => {
    void refreshList()
    const t = setInterval(() => void refreshList(), 5000)
    return () => clearInterval(t)
  }, [refreshList])

  // lightweight WS for agent desk updates
  useEffect(() => {
    const token = getToken()
    if (!token) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/im/ws?token=${encodeURIComponent(token)}`)
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        if (
          msg.type === 'conversation.updated' ||
          msg.type === 'queue.updated' ||
          msg.type === 'presence.updated' ||
          msg.type === 'conversation.read'
        ) {
          void refreshList()
        }
        if (msg.type === 'message.new' && active && msg.payload?.conversation_public_id === active.public_id) {
          const m = msg.payload.message as ImMessage
          setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]))
          if (m.sender_type !== 'agent') void markRead(active.public_id)
        }
      } catch {
        /* ignore */
      }
    }
    return () => ws.close()
  }, [active, refreshList, markRead])

  useEffect(() => {
    if (!active) return
    const wsProto = location.protocol === 'https:' ? 'wss' : 'ws'
    const token = getToken()
    if (!token) return
    const ws = new WebSocket(`${wsProto}://${location.host}/im/ws?token=${encodeURIComponent(token)}`)
    ws.onopen = () => {
      ws.send(
        JSON.stringify({
          type: 'conversation.subscribe',
          payload: { conversation_public_id: active.public_id },
        }),
      )
    }
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        if (msg.type === 'message.new' && msg.payload?.conversation_public_id === active.public_id) {
          const m = msg.payload.message as ImMessage
          setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]))
        }
        if (msg.type === 'conversation.updated' && msg.payload?.public_id === active.public_id) {
          setActive(msg.payload as ImConversation)
        }
        if (msg.type === 'conversation.read' && msg.payload?.conversation_public_id === active.public_id) {
          const { reader, seq } = msg.payload as { reader: string; seq: number }
          setActive((prev) =>
            prev
              ? reader === 'visitor'
                ? { ...prev, visitor_last_read_seq: seq }
                : { ...prev, agent_last_read_seq: seq }
              : prev,
          )
        }
      } catch {
        /* ignore */
      }
    }
    return () => ws.close()
  }, [active?.public_id])

  const title = useMemo(() => {
    if (!active) return '选择左侧会话'
    const sg = skillGroups.find((g) => g.id === active.skill_group_id)
    return `会话 ${active.public_id.slice(0, 8)}…${sg ? ` · ${sg.name}` : ''}`
  }, [active, skillGroups])

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
      message.success(`状态：${p.status}`)
      void refreshList()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '更新状态失败')
    }
  }

  async function onAccept() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await acceptConversation(active.public_id)
      setActive(conv)
      message.success('已接入')
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
      message.success('已结束')
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
      message.success(transferTarget ? '已转接坐席' : '已退回排队')
      await refreshList()
      await loadMessages(conv)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '转接失败')
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
      message.success('小结已生成')
      await loadMessages(data.conversation)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '小结失败')
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
      message.error(e instanceof Error ? e.message : '发送失败')
    }
  }

  async function onPickFile(file: File) {
    if (!active) return false
    try {
      const att = await uploadImAttachment(file)
      const m = await sendAgentAttachment(active.public_id, att, crypto.randomUUID())
      setMessages((prev) => [...prev, m])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '上传失败')
    }
    return false
  }

  const sgName = (id?: number) => skillGroups.find((g) => g.id === id)?.name || (id ? `#${id}` : '—')

  return (
    <Card
      className="im-desk-card"
      title="智能客服 · 坐席工作台"
      extra={
        <Space wrap>
          <Text type="secondary">我的状态</Text>
          <Segmented
            value={presence}
            onChange={(v) => void onPresenceChange(String(v))}
            options={[
              { label: '在线', value: 'online' },
              { label: '示忙', value: 'busy' },
              { label: '离线', value: 'offline' },
            ]}
          />
          <StatusPill
            tone={(PRESENCE_PILL[presence] ?? PRESENCE_PILL.offline).tone}
            label={(PRESENCE_PILL[presence] ?? PRESENCE_PILL.offline).label}
          />
          <Tag variant="filled" color={queueSize > 0 ? 'orange' : 'default'}>排队 {queueSize}</Tag>
          <Button href="/im/skills">技能组</Button>
          <Button href="/im/sites">站点</Button>
        </Space>
      }
      styles={{ body: { padding: 0 } }}
    >
      <Layout style={{ minHeight: 560, background: 'transparent' }}>
        <Sider width={320} className="im-desk-sider">
          <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Segmented
              block
              value={scope}
              onChange={(v) => setScope(v as Scope)}
              options={[
                { label: '全部', value: 'all' },
                { label: '我的', value: 'mine' },
                { label: '排队', value: 'queue' },
                { label: '机器人', value: 'bot' },
              ]}
            />
            <Select
              allowClear
              placeholder="按技能组筛选"
              style={{ width: '100%' }}
              value={filterSg}
              onChange={(v) => setFilterSg(v)}
              options={skillGroups.map((g) => ({ label: `${g.name} (${g.code})`, value: g.id }))}
            />
            <Button block onClick={() => void refreshList()}>
              刷新会话
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
                        <Tag variant="filled" color="purple">我</Tag>
                      ) : null}
                      <Badge count={item.unread_count || 0} size="small" />
                    </Space>
                  }
                  description={item.last_message_preview || '（尚无消息）'}
                />
              </List.Item>
            )}
          />
        </Sider>
        <Content style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="im-desk-head">
            <Text strong>{title}</Text>
            <Space wrap>
              <Tooltip title="从队列手动接入">
                <Button
                  disabled={!active || active.status === 'assigned' || active.status === 'closed'}
                  loading={loading}
                  onClick={() => void onAccept()}
                >
                  接入
                </Button>
              </Tooltip>
              <Button disabled={!active || active.status === 'closed'} loading={loading} onClick={() => void openTransfer()}>
                转接
              </Button>
              <Tooltip title="调用 AI/本地机器人生成会话小结">
                <Button disabled={!active} loading={loading} onClick={() => void onSummary()}>
                  小结
                </Button>
              </Tooltip>
              <Select
                size="small"
                style={{ width: 120 }}
                value={closeReason}
                onChange={setCloseReason}
                options={[
                  { label: '坐席结束', value: 'agent' },
                  { label: '访客结束', value: 'visitor' },
                  { label: '超时', value: 'timeout' },
                  { label: '系统', value: 'system' },
                ]}
              />
              <Button disabled={!active || active.status === 'closed'} loading={loading} danger onClick={() => void onClose()}>
                结束
              </Button>
            </Space>
          </div>
          <div className="im-desk-msgs">
            {!active && <GlassEmpty text="从左侧选择会话，或打开访客页发起咨询" />}
            {active?.summary ? (
              <div className="im-desk-summary">
                <Text strong type="secondary">
                  会话小结 ·{' '}
                </Text>
                {active.summary}
              </div>
            ) : null}
            {messages.map((m) => (
              <div key={m.id} className={`im-msg im-msg-${m.sender_type}`}>
                <div className="im-msg-meta">
                  {m.sender_type} · #{m.seq}
                </div>
                <div>{renderContent(m.content)}</div>
                {m.sender_type === 'agent' ? (
                  <div
                    className={`im-msg-read${(active?.visitor_last_read_seq ?? 0) >= m.seq ? ' is-read' : ''}`}
                  >
                    {(active?.visitor_last_read_seq ?? 0) >= m.seq ? '已读' : '未读'}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
          <div className="im-desk-input">
            <Upload
              showUploadList={false}
              beforeUpload={(f) => onPickFile(f as unknown as File)}
              accept=".jpg,.jpeg,.png,.gif,.webp,.pdf,.txt,.zip,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.csv,.mp4,.mp3"
              disabled={!active || active.status === 'closed'}
            >
              <Button icon={<PaperClipOutlined />} disabled={!active || active.status === 'closed'} title="发送图片/文件（≤10MB）" />
            </Upload>
            <Input
              value={text}
              disabled={!active || active.status === 'closed'}
              placeholder="输入回复…"
              onChange={(e) => setText(e.target.value)}
              onPressEnter={() => void onSend()}
            />
            <Button type="primary" disabled={!active || active.status === 'closed'} onClick={() => void onSend()}>
              发送
            </Button>
          </div>
          <div className="im-desk-hint">
            提示：机器人接待中可点「小结」；访客「转人工」后进入排队。坐席上线后按技能组自动接单。 ·{' '}
            <a href="/im/visitor" target="_blank" rel="noreferrer">
              访客 H5
            </a>
            {' · '}
            <a href="/im/widget/demo.html" target="_blank" rel="noreferrer">
              埋码演示
            </a>
          </div>
        </Content>
      </Layout>

      <Modal
        title="转接会话"
        open={transferOpen}
        onCancel={() => setTransferOpen(false)}
        onOk={() => void onTransfer()}
        confirmLoading={loading}
        okText={transferTarget ? '转给坐席' : '退回排队'}
      >
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <div>
            <Text type="secondary">目标坐席（不选则退回排队）</Text>
            <Select
              allowClear
              style={{ width: '100%', marginTop: 4 }}
              placeholder="选择在线/示忙坐席"
              value={transferTarget}
              onChange={setTransferTarget}
              options={agents
                .filter((a) => a.agent_user_id !== myUserId)
                .map((a) => ({
                  label: `${a.display_name || a.agent_user_id} · ${a.status} · 负载 ${a.assigned_count ?? 0}`,
                  value: a.agent_user_id,
                }))}
            />
          </div>
          <div>
            <Text type="secondary">技能组（可选，退回排队时生效）</Text>
            <Select
              allowClear
              style={{ width: '100%', marginTop: 4 }}
              placeholder="保持原技能组"
              value={transferSg}
              onChange={setTransferSg}
              options={skillGroups.map((g) => ({ label: `${g.name} (${g.strategy})`, value: g.id }))}
            />
          </div>
          <div>
            <Text type="secondary">备注</Text>
            <Input.TextArea
              style={{ marginTop: 4 }}
              rows={2}
              value={transferNote}
              onChange={(e) => setTransferNote(e.target.value)}
              placeholder="转接说明，将记入系统事件"
            />
          </div>
        </Space>
      </Modal>
    </Card>
  )
}
