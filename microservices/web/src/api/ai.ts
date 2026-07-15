import request from '@/utils/request'
import { ssePost } from '@/utils/sse'
import type {
  PageRequest,
  PageResponse,
  AiStatus,
  AiConversation,
  AiChatMessage,
  AiKbDocument,
  AiKbSearchResult,
} from '@/types'

// ===== 状态 =====

export const getAiStatus = () =>
  request.get<unknown, AiStatus>('/api/v1/ai/status', { silent: true })

// ===== 对话（SSE 流走 fetch，见 @/utils/sse）=====

export interface ChatRequest {
  conversation_id?: number
  message: string
  use_knowledge_base?: boolean
}

export type ChatStreamEvent =
  | { type: 'delta'; content: string }
  | { type: 'done'; conversation_id: number; message_id: number }
  | { type: 'error'; message: string }

export const chatStream = (
  data: ChatRequest,
  onEvent: (event: ChatStreamEvent) => void,
  signal?: AbortSignal,
) =>
  ssePost<ChatStreamEvent>({ url: '/api/v1/ai/chat', body: data, signal, onEvent })

export const getConversations = (params: PageRequest) =>
  request.get<unknown, PageResponse<AiConversation>>('/api/v1/ai/conversations', { params })

export const getConversationMessages = (id: number) =>
  request.get<unknown, { list: AiChatMessage[] }>(`/api/v1/ai/conversations/${id}/messages`)

export const deleteConversation = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/conversations/${id}`)

// ===== 知识库 =====

export const createKbDocument = (data: { title: string; content: string }) =>
  request.post<unknown, { id: number; chunk_count: number }>('/api/v1/ai/kb/documents', data)

export const getKbDocuments = (params: PageRequest) =>
  request.get<unknown, PageResponse<AiKbDocument>>('/api/v1/ai/kb/documents', { params })

export const deleteKbDocument = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/kb/documents/${id}`)

export const searchKb = (data: { query: string; top_k?: number }) =>
  request.post<unknown, { list: AiKbSearchResult[] }>('/api/v1/ai/kb/search', data)

// ===== 日志洞察 / 文案生成 =====

// AI 生成耗时明显长于普通接口，单独放宽超时
const AI_GEN_TIMEOUT = 120000

export const getLogsInsight = (data: { days?: number }) =>
  request.post<unknown, { report: string }>('/api/v1/ai/logs/insight', data, {
    timeout: AI_GEN_TIMEOUT,
  })

export const compose = (data: { kind: 'notice'; prompt: string; draft?: string }) =>
  request.post<unknown, { content: string }>('/api/v1/ai/compose', data, {
    timeout: AI_GEN_TIMEOUT,
  })
