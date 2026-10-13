import request from '@/utils/request'
import type { FsRowsResult } from './free-switch'

// ---- SIP 线路（sofia gateway）----

export type FsGateway = {
  id: number
  name: string
  username: string
  password?: string
  realm: string
  proxy: string
  register: boolean
  // From 头（显号/域），空跟随 username/realm
  from_user: string
  from_domain: string
  // 入方向落点：线路来电路由到该号码（分机/队列/IVR 接入号）
  extension: string
  register_transport: string
  expire_seconds: number
  retry_seconds: number
  // OPTIONS 保活间隔秒，0=关闭；失败自动标记线路 DOWN
  ping_seconds: number
  caller_id_in_from: boolean
  // 出局号码变换：拨 dial_prefix+号码 → 去冠 → 加 prepend → 送网关
  dial_prefix: string
  prepend: string
  // 出局主叫显号改写
  caller_number: string
  // 本线路出局并发上限（0=不限）；>0 时超限 USER_BUSY
  max_concurrent: number
  // LCR 费率选路：rate_per_min 分/分钟（0=未设，视为最贵）；同费率按 sort_weight 升序
  rate_per_min: number
  sort_weight: number
  // 线路归属租户（转售计费）：>0 时出局通话按该租户入账；0=未挂（默认账）
  tenant_id: number
  // 停用（欠费冻结联动）：注册断、字冠直拨失效；解冻租户后恢复
  disabled: boolean
  created_at: string
  // 实时注册状态：REGED 已注册 / NOREG 免注册 / TRYING 注册中 / FAIL_WAIT 失败重试 / NOT_LOADED 未加载
  state: string
}

export type FsGatewayPayload = {
  name: string
  realm: string
  proxy?: string
  username?: string
  password?: string
  register: boolean
  from_user?: string
  from_domain?: string
  extension?: string
  register_transport?: string
  expire_seconds?: number
  retry_seconds?: number
  ping_seconds?: number
  caller_id_in_from?: boolean
  dial_prefix?: string
  prepend?: string
  caller_number?: string
  max_concurrent?: number
  rate_per_min?: number
  sort_weight?: number
  tenant_id?: number
}

export function listFsGateways() {
  return request.get('/api/v1/cc/fs/gateways') as Promise<FsRowsResult<FsGateway>>
}

export function createFsGateway(data: FsGatewayPayload) {
  return request.post('/api/v1/cc/fs/gateways', data) as Promise<{ ok: boolean; applied: boolean }>
}

export function updateFsGateway(name: string, data: FsGatewayPayload) {
  return request.put(`/api/v1/cc/fs/gateways/${name}`, data) as Promise<{ ok: boolean; applied: boolean }>
}

export function deleteFsGateway(name: string) {
  return request.delete(`/api/v1/cc/fs/gateways/${name}`) as Promise<{ ok: boolean; applied: boolean }>
}

// LCR 选路预览：一条 failover 序列的一行（次序/网关/费率/送号前缀）
export type LcrPreviewRow = {
  order: number
  gateway: string
  rate_per_min: number
  prepend: string
}

export type LcrPreviewResult = {
  number: string
  matched: boolean
  list: LcrPreviewRow[]
  total: number
}

// 选路预览：给号码模拟 LCR 选路，返回按费率排好的 failover 顺序（不打 FS）
export function previewLcr(number: string) {
  return request.get('/api/v1/cc/fs/lcr-preview', { params: { number } }) as Promise<LcrPreviewResult>
}

// ---- IVR ----

export type FsIVREntry = {
  digits: string
  // transfer 转接 | submenu 进子菜单 | replay 重播 | back 返回上级 | hangup 挂断 | collect 收号
  action: 'transfer' | 'submenu' | 'replay' | 'back' | 'hangup' | 'collect'
  // transfer=目标号码；submenu=子菜单名；collect=收号参数串 "min:max:timeout:prompt:next"
  target?: string
}

export type FsIVR = {
  id: number
  name: string
  access_number: string
  greeting: string
  timeout_ms: number
  entry_list: FsIVREntry[]
  created_at: string
}

export type FsIVRPayload = {
  name: string
  access_number: string
  greeting?: string
  timeout_ms?: number
  entries: FsIVREntry[]
}

export function listFsIVRs() {
  return request.get('/api/v1/cc/fs/ivrs') as Promise<FsRowsResult<FsIVR>>
}

export function createFsIVR(data: FsIVRPayload) {
  return request.post('/api/v1/cc/fs/ivrs', data) as Promise<{ ok: boolean; reloaded: boolean }>
}

export function updateFsIVR(name: string, data: FsIVRPayload) {
  return request.put(`/api/v1/cc/fs/ivrs/${name}`, data) as Promise<{ ok: boolean; reloaded: boolean }>
}

export function deleteFsIVR(name: string) {
  return request.delete(`/api/v1/cc/fs/ivrs/${name}`) as Promise<{ ok: boolean; reloaded: boolean }>
}
