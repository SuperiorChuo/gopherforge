import request from '@/utils/request'

// ---- 呼叫控制（FS 管控台）----

// FS `show registrations as json` 的行，字段名与 FS 输出一致
export type FsRegistration = {
  reg_user: string
  realm: string
  url: string
  expires: string
  network_ip: string
  network_port: string
  network_proto: string
  hostname: string
}

// FS `show channels as json` 的行（只列用到的字段）
export type FsChannel = {
  uuid: string
  direction: string
  created: string
  name: string
  state: string
  cid_num: string
  cid_name: string
  dest: string
  callstate: string
  application: string
}

export type FsRowsResult<T> = { total: number; list: T[] }

export function listFsRegistrations() {
  return request.get('/api/v1/cc/fs/registrations') as Promise<FsRowsResult<FsRegistration>>
}

export function listFsChannels() {
  return request.get('/api/v1/cc/fs/channels') as Promise<FsRowsResult<FsChannel>>
}

// 点击外呼：先振铃 from_ext 坐席分机，接起后按拨号计划路由 to；
// 传 gateway 则经指定 SIP 线路桥接（CRM 选线路外呼）
export function fsOriginate(fromExt: string, to: string, gateway?: string) {
  return request.post('/api/v1/cc/fs/originate', { from_ext: fromExt, to, gateway: gateway || '' }) as Promise<{ queued: boolean; reply: string }>
}

// 强拆一路通道（uuid 为 FS channel uuid，即话单 call_id）
export function fsHangup(callId: string) {
  return request.post('/api/v1/cc/fs/hangup', { call_id: callId }) as Promise<{ ok: boolean; reply: string }>
}

// ---- 分机管理 ----

export type FsExtension = {
  ext: string
  password?: string
  label: string
  source: 'static' | 'dynamic'
  vm_enabled?: boolean
  vm_password?: string
  // 411 目录拼音名（如 zhang/san）；两者都空则不进按名字拨号目录
  dir_first_name?: string
  dir_last_name?: string
}

export function listFsExtensions() {
  return request.get('/api/v1/cc/fs/extensions') as Promise<FsRowsResult<FsExtension>>
}

export function createFsExtension(data: {
  ext: string; password: string; label?: string
  vm_enabled?: boolean; vm_password?: string
  dir_first_name?: string; dir_last_name?: string
}) {
  return request.post('/api/v1/cc/fs/extensions', data) as Promise<{ ok: boolean; reloaded: boolean }>
}

export function updateFsExtension(ext: string, data: {
  ext: string; password: string; label?: string
  vm_enabled?: boolean; vm_password?: string
  dir_first_name?: string; dir_last_name?: string
}) {
  return request.put(`/api/v1/cc/fs/extensions/${ext}`, data) as Promise<{ ok: boolean; reloaded: boolean }>
}

export function deleteFsExtension(ext: string) {
  return request.delete(`/api/v1/cc/fs/extensions/${ext}`) as Promise<{ ok: boolean; reloaded: boolean }>
}

// ---- 语音信箱留言 ----

export type FsVoicemail = {
  ext: string
  file: string
  size: number
  modified_at: string
}

export function listFsVoicemails(ext?: string) {
  const q = ext ? `?ext=${encodeURIComponent(ext)}` : ''
  return request.get(`/api/v1/cc/fs/voicemails${q}`) as Promise<FsRowsResult<FsVoicemail>>
}

// 留言 wav 二进制（blob 响应不走 envelope 拆包），用于 <audio> 播放
export function fetchFsVoicemailBlob(ext: string, file: string) {
  return request.get(`/api/v1/cc/fs/voicemails/${encodeURIComponent(ext)}/${encodeURIComponent(file)}`, {
    responseType: 'blob',
  }) as Promise<Blob>
}

export function deleteFsVoicemail(ext: string, file: string) {
  return request.delete(
    `/api/v1/cc/fs/voicemails/${encodeURIComponent(ext)}/${encodeURIComponent(file)}`,
  ) as Promise<{ ok: boolean }>
}

// ---- 录音 ----

export type FsRecording = {
  name: string
  size: number
  modified_at: string
}

export function listFsRecordings() {
  return request.get('/api/v1/cc/fs/recordings') as Promise<FsRowsResult<FsRecording>>
}

// 录音 wav 二进制（blob 响应不走 envelope 拆包），用于 <audio> 播放/下载
export function fetchFsRecordingBlob(name: string) {
  return request.get(`/api/v1/cc/fs/recordings/${encodeURIComponent(name)}`, {
    responseType: 'blob',
  }) as Promise<Blob>
}
