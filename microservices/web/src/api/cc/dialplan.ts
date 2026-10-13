import request from '@/utils/request'
import type { FsRowsResult } from './free-switch'

// ---- 营业时间路由（时间路由：工作日/时段分流，营业转/非营业转）----

export type CcTimeRoute = {
  id: number
  name: string
  access_number: string
  wdays: string // FS wday 语法，如 "2-6" 或 "2,4,6"
  time_ranges: string // "09:00-12:00,13:30-18:00"
  open_target: string // 营业中转（队列/IVR/分机/响铃组接入号）
  closed_target: string // 非营业转（空=播放提示音后挂断）
  closed_prompt: string // 非营业提示音文件路径（可空）
  created_at: string
}

export type CcTimeRoutePayload = {
  name: string
  access_number: string
  wdays: string
  time_ranges: string
  open_target: string
  closed_target?: string
  closed_prompt?: string
}

export function listCcTimeRoutes() {
  return request.get('/api/v1/cc/fs/time-routes') as Promise<FsRowsResult<CcTimeRoute>>
}

export function createCcTimeRoute(data: CcTimeRoutePayload) {
  return request.post('/api/v1/cc/fs/time-routes', data) as Promise<{ ok: boolean }>
}

export function updateCcTimeRoute(name: string, data: CcTimeRoutePayload) {
  return request.put(`/api/v1/cc/fs/time-routes/${name}`, data) as Promise<{ ok: boolean }>
}

export function deleteCcTimeRoute(name: string) {
  return request.delete(`/api/v1/cc/fs/time-routes/${name}`) as Promise<{ ok: boolean }>
}

// ---- 响铃组（同振/顺振一组分机，全都没接转兜底）----

export type CcRingStrategy = 'simultaneous' | 'sequential'

export type CcRingGroup = {
  id: number
  name: string
  access_number: string
  strategy: CcRingStrategy
  exts: string // 逗号分隔分机列表 "1000,1001,1002"
  ring_timeout: number
  failover_target: string // 全都没接转（空=挂断）
  created_at: string
}

export type CcRingGroupPayload = {
  name: string
  access_number: string
  strategy: CcRingStrategy
  exts: string
  ring_timeout?: number
  failover_target?: string
}

export const CC_RING_STRATEGIES: { value: CcRingStrategy; label: string }[] = [
  { value: 'simultaneous', label: '同振' },
  { value: 'sequential', label: '顺振' },
]

export function listCcRingGroups() {
  return request.get('/api/v1/cc/fs/ring-groups') as Promise<FsRowsResult<CcRingGroup>>
}

export function createCcRingGroup(data: CcRingGroupPayload) {
  return request.post('/api/v1/cc/fs/ring-groups', data) as Promise<{ ok: boolean }>
}

export function updateCcRingGroup(name: string, data: CcRingGroupPayload) {
  return request.put(`/api/v1/cc/fs/ring-groups/${name}`, data) as Promise<{ ok: boolean }>
}

export function deleteCcRingGroup(name: string) {
  return request.delete(`/api/v1/cc/fs/ring-groups/${name}`) as Promise<{ ok: boolean }>
}
