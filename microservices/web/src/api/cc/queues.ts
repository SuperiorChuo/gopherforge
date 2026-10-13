import request from '@/utils/request'
import type { FsRowsResult } from './free-switch'

// ---- callcenter 队列 ----

export type CcQueueAgent = {
  queue_name: string
  ext: string
  level: number
  position: number
  wrap_up_time: number
  max_no_answer: number
  // 拒接/示忙/无应答冷却秒（0=队列默认）
  reject_delay_time: number
  busy_delay_time: number
  no_answer_delay_time: number
  // 运行时状态
  status: string // Available / On Break / Logged Out / Unknown
  state: string // Waiting / Receiving / In a queue call
}

// 排队中客户的实时快照
export type CcQueueMember = {
  cid_number: string
  wait_sec: number
  position: number
}

export type CcQueue = {
  id: number
  name: string
  access_number: string
  strategy: string
  moh_sound: string
  max_wait_time: number
  timeout_number: string
  tier_rules_apply: boolean
  tier_rule_wait_sec: number
  // 深水区参数
  announce_sound: string
  announce_frequency: number
  record_template: boolean
  abandoned_resume_allowed: boolean
  ring_progressively_delay: number
  skip_agents_with_external_calls: boolean
  agent_no_answer_status: string
  members: number
  members_detail: CcQueueMember[]
  agents: CcQueueAgent[]
}

export type CcQueuePayload = {
  name: string
  access_number: string
  strategy?: string
  moh_sound?: string
  max_wait_time?: number
  timeout_number?: string
  tier_rules_apply?: boolean
  tier_rule_wait_sec?: number
  announce_sound?: string
  announce_frequency?: number
  record_template?: boolean
  abandoned_resume_allowed?: boolean
  ring_progressively_delay?: number
  skip_agents_with_external_calls?: boolean
  agent_no_answer_status?: string
}

// 坐席无应答自动置忙的可选状态
export const CC_NO_ANSWER_STATUSES: { value: string; label: string }[] = [
  { value: '', label: '保持不变' },
  { value: 'On Break', label: '自动示忙' },
  { value: 'Logged Out', label: '自动签出' },
]

export const CC_STRATEGIES: { value: string; label: string }[] = [
  { value: 'longest-idle-agent', label: '最久空闲优先' },
  { value: 'ring-all', label: '全体振铃' },
  { value: 'round-robin', label: '轮询' },
  { value: 'top-down', label: '自上而下' },
  { value: 'agent-with-least-talk-time', label: '通话时长最少优先' },
  { value: 'agent-with-fewest-calls', label: '接听次数最少优先' },
  { value: 'sequentially-by-agent-order', label: '按层级顺序' },
  { value: 'ring-progressively', label: '渐进振铃' },
]

export function listCcQueues() {
  return request.get('/api/v1/cc/fs/queues') as Promise<FsRowsResult<CcQueue>>
}

export function createCcQueue(data: CcQueuePayload) {
  return request.post('/api/v1/cc/fs/queues', data) as Promise<{ ok: boolean }>
}

export function updateCcQueue(name: string, data: CcQueuePayload) {
  return request.put(`/api/v1/cc/fs/queues/${name}`, data) as Promise<{ ok: boolean }>
}

export function deleteCcQueue(name: string) {
  return request.delete(`/api/v1/cc/fs/queues/${name}`) as Promise<{ ok: boolean }>
}

export function upsertCcQueueAgent(queue: string, data: {
  ext: string
  level?: number
  position?: number
  wrap_up_time?: number
  max_no_answer?: number
  reject_delay_time?: number
  busy_delay_time?: number
  no_answer_delay_time?: number
}) {
  return request.put(`/api/v1/cc/fs/queues/${queue}/agents`, data) as Promise<{ ok: boolean }>
}

export function removeCcQueueAgent(queue: string, ext: string) {
  return request.delete(`/api/v1/cc/fs/queues/${queue}/agents/${ext}`) as Promise<{ ok: boolean }>
}

// 坐席签入/示忙/签出（运行时即时生效）
export function setCcAgentStatus(ext: string, status: 'available' | 'on-break' | 'logged-out') {
  return request.post('/api/v1/cc/fs/agent-status', { ext, status }) as Promise<{ ok: boolean; status: string }>
}

// ---- 质检：监听/耳语/强插 + 通话中录音 ----

export function superviseCall(supervisorExt: string, callId: string, mode: 'listen' | 'whisper' | 'barge') {
  return request.post('/api/v1/cc/fs/supervise', {
    supervisor_ext: supervisorExt, call_id: callId, mode,
  }) as Promise<{ queued: boolean }>
}

export function uuidRecord(callId: string, action: 'start' | 'stop') {
  return request.post('/api/v1/cc/fs/uuid-record', { call_id: callId, action }) as Promise<{ ok: boolean; file: string }>
}

// ---- 排队音乐 ----

export type MohFile = {
  name: string
  size: number
  modified_at: string
  path: string
}

export function listMoh() {
  return request.get('/api/v1/cc/fs/moh') as Promise<FsRowsResult<MohFile>>
}

export function uploadMoh(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  return request.post('/api/v1/cc/fs/moh', fd, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as Promise<{ ok: boolean; name: string; path: string }>
}

export function fetchMohBlob(name: string) {
  return request.get(`/api/v1/cc/fs/moh/${encodeURIComponent(name)}`, {
    responseType: 'blob',
  }) as Promise<Blob>
}

export function deleteMoh(name: string) {
  return request.delete(`/api/v1/cc/fs/moh/${encodeURIComponent(name)}`) as Promise<{ ok: boolean }>
}

// ---- 黑名单 ----

export type BlacklistEntry = {
  id: number
  number: string
  remark: string
  created_at: string
}

export function listBlacklist() {
  return request.get('/api/v1/cc/fs/blacklist') as Promise<FsRowsResult<BlacklistEntry>>
}

export function addBlacklist(number: string, remark?: string) {
  return request.post('/api/v1/cc/fs/blacklist', { number, remark }) as Promise<{ ok: boolean }>
}

export function removeBlacklist(number: string) {
  return request.delete(`/api/v1/cc/fs/blacklist/${encodeURIComponent(number)}`) as Promise<{ ok: boolean }>
}

// ---- 班长话务监控台（wallboard）----

// 排队成员：等待时长服务端算好（wait_sec），前端不拿 epoch 自己算
export type CcMonitorMember = {
  cid_number: string
  wait_sec: number
  position: number
  state: string
}

export type CcMonitorQueue = {
  name: string
  strategy: string
  members: CcMonitorMember[]
  longest_wait_sec: number
  calls_answered: number
  calls_abandoned: number
}

export type CcMonitorAgent = {
  ext: string
  status: string // Available / On Break / Logged Out
  state: string // Waiting / Receiving / In a queue call
}

// 活动通道（子集，复用 show channels 字段）
export type CcMonitorChannel = {
  uuid: string
  direction: string
  cid_num: string
  dest: string
  callstate: string
  application: string
  created: string
}

export type CcMonitor = {
  queues: CcMonitorQueue[]
  agents: CcMonitorAgent[]
  channels: CcMonitorChannel[]
  ts: number
}

// 一次拉全量盯盘数据（队列 + 坐席 + 通道）
export function getCcMonitor() {
  return request.get('/api/v1/cc/fs/monitor', { silent: true }) as Promise<CcMonitor>
}
