import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  Empty,
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
} from 'antd'
import { message } from '@/utils/feedback'
import {
  acceptConversation,
  closeConversation,
  getAgentMe,
  listAgentConversations,
  listMessages,
  listOnlineAgents,
  listSkillGroups,
  sendAgentMessage,
  setAgentPresence,
  summarizeConversation,
  transferConversation,
  type ImConversation,
  type ImMessage,
  type ImPresence,
  type ImSkillGroup,
} from '@/api/im'
import { getToken } from '@/utils/request'

const { Sider, Content } = Layout
const { Text } = Typography

function parseContent(content: string) {
  try {
    const o = JSON.parse(content)
    if (o.event) return `[系统] ${o.event}${o.note ? ` · ${o.note}` : ''}`
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

function presenceColor(s: string) {
  if (s === 'online') return 'green'
  if (s === 'busy') return 'gold'
  return 'default'
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
        if (msg.type === 'conversation.updated' || msg.type === 'queue.updated' || msg.type === 'presence.updated') {
          void refreshList()
        }
        if (msg.type === 'message.new' && active && msg.payload?.conversation_public_id === active.public_id) {
          const m = msg.payload.message as ImMessage
          setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]))
        }
      } catch {
        /* ignore */
      }
    }
    return () => ws.close()
  }, [active, refreshList])

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

  const sgName = (id?: number) => skillGroups.find((g) => g.id === id)?.name || (id ? `#${id}` : '—')

  return (
    <Card
      title="智能客服 · 坐席工作台 (IM M4)"
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
          <Tag color={presenceColor(presence)}>{presence}</Tag>
          <Tag color="orange">排队 {queueSize}</Tag>
          <Button href="/im/skills">技能组</Button>
          <Button href="/im/sites">站点</Button>
        </Space>
      }
      styles={{ body: { padding: 0 } }}
    >
      <Layout style={{ minHeight: 560, background: 'transparent' }}>
        <Sider width={320} theme="light" style={{ borderRight: '1px solid rgba(0,0,0,.06)' }}>
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
            locale={{ emptyText: <Empty description="暂无会话" /> }}
            renderItem={(item) => (
              <List.Item
                style={{
                  cursor: 'pointer',
                  padding: '10px 16px',
                  background: active?.public_id === item.public_id ? 'rgba(99,102,241,.08)' : undefined,
                }}
                onClick={() => void onSelect(item)}
              >
                <List.Item.Meta
                  title={
                    <Space wrap size={4}>
                      <Text code>{item.public_id.slice(0, 8)}</Text>
                      <Tag color={statusColor(item.status)}>{item.status}</Tag>
                      {item.skill_group_id ? (
                        <Tag>{sgName(item.skill_group_id)}</Tag>
                      ) : null}
                      {item.agent_user_id && item.agent_user_id === myUserId ? (
                        <Tag color="purple">我</Tag>
                      ) : null}
                    </Space>
                  }
                  description={item.last_message_preview || '（尚无消息）'}
                />
              </List.Item>
            )}
          />
        </Sider>
        <Content style={{ display: 'flex', flexDirection: 'column' }}>
          <div
            style={{
              padding: '12px 16px',
              borderBottom: '1px solid rgba(0,0,0,.06)',
              display: 'flex',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: 8,
            }}
          >
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
          <div style={{ flex: 1, overflow: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {!active && <Empty description="从左侧选择会话，或打开访客页发起咨询" />}
            {active?.summary ? (
              <div
                style={{
                  padding: '8px 12px',
                  borderRadius: 8,
                  background: 'rgba(124,58,237,.08)',
                  fontSize: 13,
                  border: '1px solid rgba(124,58,237,.2)',
                }}
              >
                <Text strong type="secondary">
                  会话小结 ·{' '}
                </Text>
                {active.summary}
              </div>
            ) : null}
            {messages.map((m) => (
              <div
                key={m.id}
                style={{
                  alignSelf:
                    m.sender_type === 'agent' ? 'flex-end' : m.sender_type === 'system' ? 'center' : 'flex-start',
                  maxWidth: m.sender_type === 'system' ? '90%' : '70%',
                  padding: '8px 12px',
                  borderRadius: 12,
                  background:
                    m.sender_type === 'agent'
                      ? 'rgba(99,102,241,.9)'
                      : m.sender_type === 'bot'
                        ? 'rgba(124,58,237,.12)'
                        : m.sender_type === 'system'
                          ? 'rgba(0,0,0,.04)'
                          : 'rgba(0,0,0,.06)',
                  color: m.sender_type === 'agent' ? '#fff' : undefined,
                  fontSize: m.sender_type === 'system' ? 12 : undefined,
                }}
              >
                <div style={{ fontSize: 12, opacity: 0.75, marginBottom: 4 }}>
                  {m.sender_type} · #{m.seq}
                </div>
                <div>{parseContent(m.content)}</div>
              </div>
            ))}
          </div>
          <div style={{ padding: 12, borderTop: '1px solid rgba(0,0,0,.06)', display: 'flex', gap: 8 }}>
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
          <div style={{ padding: '0 12px 12px', fontSize: 12, opacity: 0.65 }}>
            提示：机器人接待中可点「小结」；访客「转人工」后进入排队。坐席上线后按技能组自动接单。 ·{' '}
            <a href="/im/visitor" target="_blank" rel="noreferrer">
              访客 H5
            </a>
            {' · '}
            <a href="/im/widget/demo" target="_blank" rel="noreferrer">
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
