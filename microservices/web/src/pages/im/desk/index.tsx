import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Card, Empty, Input, Layout, List, Space, Tag, Typography } from 'antd'
import { message } from '@/utils/feedback'
import {
  acceptConversation,
  closeConversation,
  listAgentConversations,
  listMessages,
  sendAgentMessage,
  type ImConversation,
  type ImMessage,
} from '@/api/im'
import { getToken } from '@/utils/request'

const { Sider, Content } = Layout
const { Text } = Typography

function parseContent(content: string) {
  try {
    const o = JSON.parse(content)
    return o.text || content
  } catch {
    return content
  }
}

function statusColor(s: string) {
  if (s === 'queued') return 'orange'
  if (s === 'assigned') return 'blue'
  if (s === 'closed') return 'default'
  return 'default'
}

export default function ImDeskPage() {
  const [list, setList] = useState<ImConversation[]>([])
  const [active, setActive] = useState<ImConversation | null>(null)
  const [messages, setMessages] = useState<ImMessage[]>([])
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)

  const refreshList = useCallback(async () => {
    try {
      const data = await listAgentConversations()
      setList(data.list || [])
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载会话失败')
    }
  }, [])

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
        if (msg.type === 'conversation.updated') {
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
      } catch {
        /* ignore */
      }
    }
    return () => ws.close()
  }, [active?.public_id])

  const title = useMemo(() => (active ? `会话 ${active.public_id.slice(0, 8)}…` : '选择左侧会话'), [active])

  async function onSelect(item: ImConversation) {
    setActive(item)
    await loadMessages(item)
  }

  async function onAccept() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await acceptConversation(active.public_id)
      setActive(conv)
      message.success('已接入')
      await refreshList()
    } finally {
      setLoading(false)
    }
  }

  async function onClose() {
    if (!active) return
    setLoading(true)
    try {
      const conv = await closeConversation(active.public_id)
      setActive(conv)
      message.success('已结束')
      await refreshList()
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

  return (
    <Card title="智能客服 · 坐席工作台 (IM M1)" styles={{ body: { padding: 0 } }}>
      <Layout style={{ minHeight: 560, background: 'transparent' }}>
        <Sider width={300} theme="light" style={{ borderRight: '1px solid rgba(0,0,0,.06)' }}>
          <div style={{ padding: 12 }}>
            <Button block onClick={() => void refreshList()}>
              刷新会话
            </Button>
          </div>
          <List
            dataSource={list}
            locale={{ emptyText: <Empty description="暂无排队/进行中会话" /> }}
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
                    <Space>
                      <Text code>{item.public_id.slice(0, 8)}</Text>
                      <Tag color={statusColor(item.status)}>{item.status}</Tag>
                    </Space>
                  }
                  description={item.last_message_preview || '（尚无消息）'}
                />
              </List.Item>
            )}
          />
        </Sider>
        <Content style={{ display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: '12px 16px', borderBottom: '1px solid rgba(0,0,0,.06)', display: 'flex', justifyContent: 'space-between' }}>
            <Text strong>{title}</Text>
            <Space>
              <Button disabled={!active || active.status === 'assigned'} loading={loading} onClick={() => void onAccept()}>
                接入
              </Button>
              <Button disabled={!active || active.status === 'closed'} loading={loading} danger onClick={() => void onClose()}>
                结束
              </Button>
            </Space>
          </div>
          <div style={{ flex: 1, overflow: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {!active && <Empty description="从左侧选择会话，或打开访客页发起咨询" />}
            {messages.map((m) => (
              <div
                key={m.id}
                style={{
                  alignSelf: m.sender_type === 'agent' ? 'flex-end' : 'flex-start',
                  maxWidth: '70%',
                  padding: '8px 12px',
                  borderRadius: 12,
                  background: m.sender_type === 'agent' ? 'rgba(99,102,241,.9)' : 'rgba(0,0,0,.06)',
                  color: m.sender_type === 'agent' ? '#fff' : undefined,
                }}
              >
                <div style={{ fontSize: 12, opacity: 0.75, marginBottom: 4 }}>{m.sender_type} · #{m.seq}</div>
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
            访客 H5（经网关）：
            <a href="/im/visitor" target="_blank" rel="noreferrer">
              /im/visitor
            </a>
          </div>
        </Content>
      </Layout>
    </Card>
  )
}
