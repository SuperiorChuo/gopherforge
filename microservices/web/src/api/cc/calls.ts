import request from '@/utils/request'

export const HANGUP_CLASS_LABEL: Record<string, string> = {
  ok: '正常挂断',
  empty: '空号',
  busy: '忙线',
  noanswer: '无人接听',
  unavailable: '无法接通',
  rejected: '拒接',
  local: '本端挂断',
  other: '其他',
}

export type CcCall = {
  id: number
  tenant_id: number
  call_id: string
  direction: string
  caller: string
  callee: string
  agent_ext: string
  queue: string
  status: string
  hangup_cause: string
  hangup_class?: string
  duration_sec: number
  recording: string
  // IVR 收号（play_and_get_digits）随 CDR 带出的按键串，可空
  collected_digits?: string
  started_at?: string
  answered_at?: string
  ended_at?: string
  created_at: string
  updated_at: string
}

export type CcCallListParams = {
  page: number
  page_size: number
  status?: string
  caller?: string
  agent_ext?: string
  // 号码双向匹配（caller 或 callee）
  number?: string
}

export type CcCallListResult = {
  list: CcCall[]
  total: number
  page: number
  page_size: number
}

// FS 状态代理：configured=false 表示 CC_FS_API_BASE 未配置
export type CcFsStatus = {
  configured: boolean
  reachable?: boolean
  error?: string
  status?: Record<string, unknown>
}

export function listCcCalls(params: CcCallListParams) {
  return request.get('/api/v1/cc/calls', { params }) as Promise<CcCallListResult>
}

// ---- 号码识客聚合（坐席工作台来电弹屏）----

// 识客卡里的 CRM 客户子集（cc lookup 聚合的 crm_customer 字段，对齐 crm 模型）
export type CcLookupCrmCustomer = {
  id: number
  name: string
  phone: string
  level?: string
  industry?: string
  owner_user_id?: number
}

// 识客卡里的 CRM 线索子集
export type CcLookupCrmLead = {
  id: number
  name: string
  phone: string
  status?: string
}

// 近 5 通历史精简行
export type CcLookupRecentCall = {
  call_id: string
  direction: string
  status: string
  duration_sec: number
  agent_ext: string
  started_at?: string
}

// GET /api/v1/cc/lookup?number= 聚合返回
export type CcLookup = {
  number: string
  contact: { name: string; remark: string } | null
  crm_customer: CcLookupCrmCustomer | null
  crm_lead: CcLookupCrmLead | null
  has_crm: boolean
  recent_calls: CcLookupRecentCall[]
}

// 号码识客：cc 联系人备注 + CRM 客户/线索归属 + 近 5 通历史，一次聚合返回
export function lookupCcNumber(number: string) {
  return request.get('/api/v1/cc/lookup', { params: { number } }) as Promise<CcLookup>
}

// ---- 坐席分机绑定（cc_agents：用户 ↔ 分机）----

export type CcAgent = {
  id: number
  tenant_id: number
  user_id: number
  extension: string
  display_name: string
  created_at: string
  updated_at: string
}

// 列出本租户所有坐席绑定（工作台找当前用户的绑定分机）
export function listCcAgents() {
  return request.get('/api/v1/cc/agents') as Promise<{ list: CcAgent[]; total: number }>
}

// 绑定/更新某用户的分机（工作台「绑定我的分机」）
export function bindCcAgent(userId: number, extension: string, displayName?: string) {
  return request.put(`/api/v1/cc/agents/${userId}`, {
    extension, display_name: displayName || '',
  }) as Promise<CcAgent>
}

export function unbindCcAgent(userId: number) {
  return request.delete(`/api/v1/cc/agents/${userId}`) as Promise<{ removed: boolean; user_id: number }>
}

export function getCcFsStatus() {
  return request.get('/api/v1/cc/fs/status') as Promise<CcFsStatus>
}

export function testCcWebhook() {
  return request.post('/api/v1/cc/fs/webhook-test', {}) as Promise<Record<string, unknown>>
}
