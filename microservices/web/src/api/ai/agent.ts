import request from '@/utils/request'
import { sseGet, type SseFrame } from '@/utils/sse'

export type AgentDefinitionStatus = 'active' | 'disabled'
export type AgentRunStatus = 'queued' | 'running' | 'cancelling' | 'succeeded' | 'failed' | 'cancelled'
export type AgentStepStatus = 'running' | 'succeeded' | 'failed' | 'cancelled'

export interface AgentDefinition {
  id: number
  tenant_id: number
  agent_key: string
  name: string
  description: string
  status: AgentDefinitionStatus
  latest_version_id?: number
  created_by: number
  created_at: string
  updated_at: string
}

export interface AgentVersion {
  id: number
  tenant_id: number
  agent_definition_id: number
  version: number
  schema_version: 'agent-spec/v1'
  model_id: number
  instructions: string
  tools: string[]
  max_steps: number
  timeout_seconds: number
  spec_sha256: string
  created_by: number
  created_at: string
}

export interface AgentRun {
  id: number
  tenant_id: number
  agent_definition_id: number
  agent_version_id: number
  requested_by: number
  idempotency_key: string
  input: string
  status: AgentRunStatus
  attempt: number
  max_steps: number
  timeout_seconds: number
  output: string
  error_code: string
  error_message: string
  lease_expires_at?: string
  heartbeat_at?: string
  cancel_requested_at?: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface AgentStep {
  id: number
  tenant_id: number
  run_id: number
  ordinal: number
  kind: 'model' | 'tool'
  status: AgentStepStatus
  name: string
  request: unknown
  response: unknown
  input_tokens: number
  output_tokens: number
  stop_reason: string
  error_code: string
  error_message: string
  started_at: string
  finished_at?: string
}

export interface AgentToolCall {
  id: number
  run_id: number
  step_id: number
  provider_call_id: string
  tool_name: string
  arguments: unknown
  status: AgentStepStatus
  result: unknown
  attempt: number
  started_at: string
  finished_at?: string
}

export interface AgentRunEvent {
  id: number
  tenant_id: number
  run_id: number
  step_id?: number
  schema: 'run-event/v1'
  type: string
  data: Record<string, unknown>
  occurred_at: string
}

export interface AgentRunDetails {
  run: AgentRun
  definition: AgentDefinition
  version: AgentVersion
  steps: AgentStep[]
  tool_calls: AgentToolCall[]
  events: AgentRunEvent[]
  events_has_more: boolean
  events_next_cursor: number
}

export interface AgentToolDefinition {
  schema: 'tool-spec/v1'
  name: string
  description: string
  read_only: boolean
  timeout_ms: number
  input_schema: Record<string, unknown>
}

export interface PageResult<T> {
  list: T[]
  total: number
}

export const listAgentDefinitions = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResult<AgentDefinition>>('/api/v1/ai/agent-kernel/agents', { params })

export const createAgentDefinition = (data: { agent_key: string; name: string; description?: string }) =>
  request.post<unknown, AgentDefinition>('/api/v1/ai/agent-kernel/agents', data)

export const getAgentDefinition = (id: number) =>
  request.get<unknown, { definition: AgentDefinition; versions: AgentVersion[] }>(`/api/v1/ai/agent-kernel/agents/${id}`)

export const updateAgentDefinition = (id: number, data: { name: string; description: string; status: AgentDefinitionStatus }) =>
  request.patch<unknown, AgentDefinition>(`/api/v1/ai/agent-kernel/agents/${id}`, data)

export const publishAgentVersion = (id: number, data: {
  instructions: string
  model_id: number
  tools: string[]
  max_steps: number
  timeout_seconds: number
}) => request.post<unknown, AgentVersion>(`/api/v1/ai/agent-kernel/agents/${id}/versions`, data)

export const listAgentTools = () =>
  request.get<unknown, AgentToolDefinition[]>('/api/v1/ai/agent-kernel/tools')

export const createAgentRun = (agentID: number, input: string, idempotencyKey: string) =>
  request.post<unknown, { run: AgentRun; reused: boolean }>(
    `/api/v1/ai/agent-kernel/agents/${agentID}/runs`,
    { input },
    { headers: { 'Idempotency-Key': idempotencyKey } },
  )

export const listAgentRuns = (params: { agent_id?: number; status?: AgentRunStatus; page: number; page_size: number }) =>
  request.get<unknown, PageResult<AgentRun>>('/api/v1/ai/agent-kernel/runs', { params })

// after_id是已展示的末条事件ID，不以数组长度或页码替代全局账本游标。
export const getAgentRun = (id: number, afterID = 0) =>
  request.get<unknown, AgentRunDetails>(`/api/v1/ai/agent-kernel/runs/${id}`, { params: { after_id: afterID } })

export const cancelAgentRun = (id: number) =>
  request.post<unknown, AgentRun>(`/api/v1/ai/agent-kernel/runs/${id}/cancel`)

export type AgentEventStreamPayload = AgentRunEvent | { run_id: number } | { message: string }

export const watchAgentRunEvents = (
  id: number,
  lastEventID: number | undefined,
  signal: AbortSignal,
  onEvent: (frame: SseFrame<AgentEventStreamPayload>) => void,
) => sseGet({
  url: `/api/v1/ai/agent-kernel/runs/${id}/events`,
  lastEventId: lastEventID,
  signal,
  onEvent,
})
