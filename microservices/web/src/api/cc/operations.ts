import request from '@/utils/request'
import type { FsRowsResult } from './free-switch'

// ---- 会议室（固定/动态会议室、成员管控、会议录音）----

export type CcConferenceMember = {
  member_id: string
  caller_id_name: string
  caller_id_number: string
  muted: boolean
}

export type CcConference = {
  id: number
  name: string
  number: string
  pin: string
  max_members: number
  auto_record: boolean
  description: string
  // 实时态（conference xml_list 合并）
  active: boolean
  member_count: number
  recording: boolean
  locked: boolean
  dynamic: boolean
  members: CcConferenceMember[]
}

export type CcConferencePayload = {
  name: string
  number: string
  pin?: string
  max_members?: number
  auto_record?: boolean
  description?: string
}

export type CcConferenceAction =
  | 'mute' | 'unmute' | 'deaf' | 'undeaf' | 'kick'
  | 'lock' | 'unlock' | 'record_start' | 'record_stop'

export function listCcConferences() {
  return request.get('/api/v1/cc/fs/conferences') as Promise<FsRowsResult<CcConference>>
}

export function createCcConference(data: CcConferencePayload) {
  return request.post('/api/v1/cc/fs/conferences', data) as Promise<{ ok: boolean }>
}

export function updateCcConference(name: string, data: CcConferencePayload) {
  return request.put(`/api/v1/cc/fs/conferences/${name}`, data) as Promise<{ ok: boolean }>
}

export function deleteCcConference(name: string) {
  return request.delete(`/api/v1/cc/fs/conferences/${name}`) as Promise<{ ok: boolean }>
}

// 成员/会议管控（mute/unmute/deaf/undeaf/kick 需 member_id；lock/unlock/record_* 针对整会议）
export function ccConferenceAction(name: string, action: CcConferenceAction, memberId?: string) {
  return request.post(`/api/v1/cc/fs/conferences/${name}/action`, {
    action,
    ...(memberId ? { member_id: memberId } : {}),
  }) as Promise<{ ok: boolean; output?: string }>
}

// ---- 坐席通话操控：转移 / 保持 / 停泊 / DTMF ----

// 转移通话到目标号码（走 default 拨号计划）。both=true 双方一起转
export function transferCall(callId: string, to: string, both = false) {
  return request.post('/api/v1/cc/fs/transfer', {
    call_id: callId, to, both,
  }) as Promise<{ ok: boolean; reply: string }>
}

// 通话保持/恢复/切换（uuid_hold）
export function holdCall(callId: string, action: 'on' | 'off' | 'toggle') {
  return request.post('/api/v1/cc/fs/hold', {
    call_id: callId, action,
  }) as Promise<{ ok: boolean; reply: string }>
}

// 把通话停泊到指定车位（6100-6199），任意分机拨该号码取回
export function parkCall(callId: string, slot: string) {
  return request.post('/api/v1/cc/fs/park', {
    call_id: callId, slot,
  }) as Promise<{ ok: boolean; slot: string; reply: string }>
}

// 向通话发送 DTMF 按键（uuid_send_dtmf）
export function sendDtmf(callId: string, digits: string) {
  return request.post('/api/v1/cc/fs/dtmf', {
    call_id: callId, digits,
  }) as Promise<{ ok: boolean; reply: string }>
}

// 一条停泊记录
export type ParkedSlot = {
  slot: string
  caller: string
}

// 列出停泊车位里的通话（valet_info cclot 解析）
export function listParked() {
  return request.get('/api/v1/cc/fs/parked') as Promise<FsRowsResult<ParkedSlot>>
}

// ---- 提示音库（上传 + TTS 预生成）----

export type PromptFile = {
  name: string
  size: number
  modified_at: string
  path: string
}

export function listPrompts() {
  return request.get('/api/v1/cc/fs/prompts') as Promise<FsRowsResult<PromptFile>>
}

export function uploadPrompt(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  return request.post('/api/v1/cc/fs/prompts', fd, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as Promise<{ ok: boolean; name: string; path: string }>
}

export function fetchPromptBlob(name: string) {
  return request.get(`/api/v1/cc/fs/prompts/${encodeURIComponent(name)}`, {
    responseType: 'blob',
  }) as Promise<Blob>
}

export function deletePrompt(name: string) {
  return request.delete(`/api/v1/cc/fs/prompts/${encodeURIComponent(name)}`) as Promise<{ ok: boolean }>
}

// TTS 预生成：文本转 8k 单声道 wav 落库（管理面预生成，不动 FS 运行时）
export function generatePromptTTS(data: { name: string; text: string; voice?: string }) {
  return request.post('/api/v1/cc/fs/prompts/tts', data) as Promise<{ ok: boolean; name: string; path: string }>
}
