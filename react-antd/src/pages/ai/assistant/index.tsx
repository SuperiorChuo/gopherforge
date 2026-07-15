import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, Button, Card, Input, Popconfirm, Spin, Switch, Tooltip } from 'antd'
import {
  DeleteOutlined,
  DatabaseOutlined,
  LoadingOutlined,
  PlusOutlined,
  RobotOutlined,
  SendOutlined,
  StopOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import * as AiAPI from '@/api/ai'
import type { AiConversation, AiStatus } from '@/types'
import AiMarkdown from '@/components/AiMarkdown'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'

interface LocalMessage {
  key: string
  role: 'user' | 'assistant'
  content: string
  error?: boolean
}

let msgSeq = 0
const nextKey = () => `local-${Date.now()}-${msgSeq++}`

export default function AiAssistantPage() {
  const [status, setStatus] = useState<AiStatus | null>(null)
  const [conversations, setConversations] = useState<AiConversation[]>([])
  const [convLoading, setConvLoading] = useState(false)
  const [activeId, setActiveId] = useState<number | null>(null)
  const [messages, setMessages] = useState<LocalMessage[]>([])
  const [msgLoading, setMsgLoading] = useState(false)
  const [input, setInput] = useState('')
  const [useKb, setUseKb] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  // 切换会话后旧流的回调不应再写入消息区
  const activeIdRef = useRef<number | null>(null)
  activeIdRef.current = activeId

  useEffect(() => {
    AiAPI.getAiStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  const fetchConversations = useCallback(async () => {
    setConvLoading(true)
    try {
      const res = await AiAPI.getConversations({ page: 1, page_size: 100 })
      setConversations(res.list)
    } catch {
      // 拦截器已提示
    } finally {
      setConvLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchConversations()
  }, [fetchConversations])

  // 组件卸载时停掉进行中的流
  useEffect(() => () => abortRef.current?.abort(), [])

  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages])

  const stopStreaming = () => {
    abortRef.current?.abort()
    abortRef.current = null
    setStreaming(false)
  }

  const switchConversation = async (id: number | null) => {
    if (streaming) stopStreaming()
    setActiveId(id)
    if (id === null) {
      setMessages([])
      return
    }
    setMsgLoading(true)
    try {
      const res = await AiAPI.getConversationMessages(id)
      setMessages(
        res.list.map((m) => ({ key: `srv-${m.id}`, role: m.role, content: m.content })),
      )
    } catch {
      setMessages([])
    } finally {
      setMsgLoading(false)
    }
  }

  const handleDeleteConversation = async (id: number) => {
    try {
      await AiAPI.deleteConversation(id)
      message.success('会话已删除')
      if (id === activeId) {
        if (streaming) stopStreaming()
        setActiveId(null)
        setMessages([])
      }
      fetchConversations()
    } catch {
      // 拦截器已提示
    }
  }

  const handleSend = async () => {
    const text = input.trim()
    if (!text || streaming) return
    const conversationBeforeSend = activeId

    setInput('')
    const assistantKey = nextKey()
    setMessages((prev) => [
      ...prev,
      { key: nextKey(), role: 'user', content: text },
      { key: assistantKey, role: 'assistant', content: '' },
    ])
    setStreaming(true)

    const controller = new AbortController()
    abortRef.current = controller

    const appendToAssistant = (updater: (m: LocalMessage) => LocalMessage) => {
      // 用户中途切换了会话就丢弃后续回调
      if (activeIdRef.current !== conversationBeforeSend && abortRef.current !== controller) return
      setMessages((prev) => prev.map((m) => (m.key === assistantKey ? updater(m) : m)))
    }

    try {
      await AiAPI.chatStream(
        {
          message: text,
          use_knowledge_base: useKb,
          ...(conversationBeforeSend ? { conversation_id: conversationBeforeSend } : {}),
        },
        (event) => {
          if (event.type === 'delta') {
            appendToAssistant((m) => ({ ...m, content: m.content + event.content }))
          } else if (event.type === 'done') {
            if (!conversationBeforeSend) {
              setActiveId(event.conversation_id)
            }
            fetchConversations()
          } else if (event.type === 'error') {
            appendToAssistant((m) => ({
              ...m,
              error: true,
              content: m.content || event.message || 'AI 服务返回错误',
            }))
            message.error(event.message || 'AI 服务返回错误')
          }
        },
        controller.signal,
      )
    } catch (err) {
      if ((err as Error)?.name !== 'AbortError') {
        const msg = (err as Error)?.message || '发送失败'
        appendToAssistant((m) => ({ ...m, error: true, content: m.content || msg }))
        message.error(msg)
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null
        setStreaming(false)
      }
    }
  }

  return (
    <div className="ai-chat-page">
      {status && !status.configured && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message="AI 服务未配置"
          description="未检测到可用的模型凭证，请在服务端配置 AI_API_KEY 后重启 ai-service。"
        />
      )}

      <div className="ai-chat-layout">
        {/* 左侧：会话列表 */}
        <Card className="ai-conv-panel" styles={{ body: { padding: 12, height: '100%', display: 'flex', flexDirection: 'column' } }}>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            block
            style={{ marginBottom: 12 }}
            onClick={() => switchConversation(null)}
          >
            新建会话
          </Button>
          <Spin spinning={convLoading}>
            <div className="ai-conv-list">
              {conversations.length === 0 && !convLoading ? (
                <GlassEmpty text="暂无历史会话" compact />
              ) : (
                conversations.map((c) => (
                  <div
                    key={c.id}
                    className={`ai-conv-item ${c.id === activeId ? 'ai-conv-item-active' : ''}`}
                    onClick={() => c.id !== activeId && switchConversation(c.id)}
                  >
                    <div className="ai-conv-item-main">
                      <div className="ai-conv-item-title">{c.title || `会话 #${c.id}`}</div>
                      <div className="ai-conv-item-time">{formatDateTime(c.updated_at || c.created_at)}</div>
                    </div>
                    <Popconfirm
                      title="删除该会话?"
                      onConfirm={() => handleDeleteConversation(c.id)}
                      onPopupClick={(e) => e.stopPropagation()}
                    >
                      <Button
                        type="text"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        className="ai-conv-item-del"
                        onClick={(e) => e.stopPropagation()}
                      />
                    </Popconfirm>
                  </div>
                ))
              )}
            </div>
          </Spin>
        </Card>

        {/* 右侧：消息流 + 输入区 */}
        <Card className="ai-msg-panel" styles={{ body: { padding: 0, height: '100%', display: 'flex', flexDirection: 'column' } }}>
          <div className="ai-msg-scroll" ref={scrollRef}>
            {msgLoading ? (
              <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 80 }}>
                <Spin />
              </div>
            ) : messages.length === 0 ? (
              <div className="ai-msg-welcome">
                <div className="ai-msg-welcome-icon"><RobotOutlined /></div>
                <div className="ai-msg-welcome-title">AI 助手</div>
                <div className="ai-msg-welcome-sub">
                  {status?.configured
                    ? `由 ${status.provider} · ${status.chat_model} 驱动${status.kb_documents > 0 ? `，知识库已收录 ${status.kb_documents} 篇文档` : ''}`
                    : '输入问题开始对话，支持知识库增强检索'}
                </div>
              </div>
            ) : (
              messages.map((m) => (
                <div key={m.key} className={`ai-msg-row ai-msg-row-${m.role}`}>
                  <span className={`ai-msg-avatar ai-msg-avatar-${m.role}`}>
                    {m.role === 'user' ? <UserOutlined /> : <RobotOutlined />}
                  </span>
                  <div className={`ai-msg-bubble ai-msg-bubble-${m.role} ${m.error ? 'ai-msg-bubble-error' : ''}`}>
                    {m.role === 'assistant' ? (
                      m.content === '' && streaming ? (
                        <span className="ai-msg-typing"><LoadingOutlined /> 思考中…</span>
                      ) : (
                        <AiMarkdown content={m.content} />
                      )
                    ) : (
                      <div className="ai-msg-user-text">{m.content}</div>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="ai-input-bar">
            <div className="ai-input-tools">
              <Tooltip title="开启后，回答会先检索知识库文档作为上下文">
                <span className="ai-kb-switch">
                  <DatabaseOutlined />
                  <span>知识库增强</span>
                  <Switch size="small" checked={useKb} onChange={setUseKb} />
                </span>
              </Tooltip>
            </div>
            <div className="ai-input-row">
              <Input.TextArea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder={status && !status.configured ? 'AI 服务未配置，暂不可用' : '输入消息，Enter 发送，Shift+Enter 换行'}
                autoSize={{ minRows: 1, maxRows: 5 }}
                disabled={!!status && !status.configured}
                onPressEnter={(e) => {
                  if (!e.shiftKey) {
                    e.preventDefault()
                    handleSend()
                  }
                }}
              />
              {streaming ? (
                <Button danger icon={<StopOutlined />} onClick={stopStreaming}>
                  停止
                </Button>
              ) : (
                <Button
                  type="primary"
                  icon={<SendOutlined />}
                  disabled={!input.trim() || (!!status && !status.configured)}
                  onClick={handleSend}
                >
                  发送
                </Button>
              )}
            </div>
          </div>
        </Card>
      </div>
    </div>
  )
}
