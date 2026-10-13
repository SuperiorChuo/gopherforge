import request from '@/utils/request'

export type CcRiskPolicy = {
  id: number
  tenant_id: number
  outbound_start: string
  outbound_end: string
  outbound_weekdays: string
  max_calls_per_number_per_hour: number
  max_calls_per_agent_per_hour: number
  max_calls_per_tenant_per_min: number
  blocked_prefixes: string
  max_rate_per_min: number
  created_at: string
  updated_at: string
}

export type CcRiskPolicyPayload = {
  outbound_start: string
  outbound_end: string
  outbound_weekdays: string
  max_calls_per_number_per_hour: number
  max_calls_per_agent_per_hour: number
  max_calls_per_tenant_per_min: number
  blocked_prefixes: string
  max_rate_per_min: number
}

export type CcRiskListEntry = {
  id: number
  tenant_id: number
  number: string
  match: 'exact' | 'prefix'
  list_type: 'black' | 'white'
  remark: string
  created_at: string
}

export function getCcRiskPolicy() {
  return request.get('/api/v1/cc/risk/policy') as Promise<CcRiskPolicy>
}

export function saveCcRiskPolicy(payload: CcRiskPolicyPayload) {
  return request.put('/api/v1/cc/risk/policy', payload) as Promise<CcRiskPolicy>
}

export function listCcRiskLists() {
  return request.get('/api/v1/cc/risk/lists') as Promise<{ list: CcRiskListEntry[]; total: number }>
}

export function createCcRiskList(payload: {
  number: string
  match: 'exact' | 'prefix'
  list_type: 'black' | 'white'
  remark?: string
}) {
  return request.post('/api/v1/cc/risk/lists', payload) as Promise<CcRiskListEntry>
}

export function deleteCcRiskList(id: number) {
  return request.delete(`/api/v1/cc/risk/lists/${id}`) as Promise<{ deleted: boolean }>
}
