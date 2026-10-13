import request from '@/utils/request'
import type { FsRowsResult } from './free-switch'

// ---- 录音转写质检 ----

// 一条命中明细：违禁词命中(count>0) / 必说词缺失(type=required,count=0)
export type QcHit = {
  word: string
  type: 'forbidden' | 'required'
  count: number
}

// 转写任务（转写在后台跑：整段 wav 不再进请求内存，接口毫秒级返回）
export type QcTranscribeJob = {
  id: number
  call_id: string
  status: 'pending' | 'running' | 'done' | 'failed'
  attempts: number
  max_attempts: number
  error?: string
  score?: number
  started_at?: string
  finished_at?: string
  created_at: string
}

// 入队应答（202）
export type QcTranscribeAccepted = {
  job_id: number
  call_id: string
  status: QcTranscribeJob['status']
  created: boolean // false = 复用了已在进行中的同通话任务
}

// 任务状态查询：done 时带上文本与质检结论
export type QcTranscribeResult = {
  job: QcTranscribeJob
  text?: string
  segments?: string
  score?: number
  hits?: QcHit[]
}

export type QcTranscript = {
  id: number
  call_id: string
  text: string
  segments?: string
  status: 'pending' | 'done' | 'failed'
  error?: string
  created_at: string
}

export type QcKeyword = {
  id: number
  word: string
  type: 'forbidden' | 'required'
  remark: string
  created_at: string
}

// 质检结果行（联 call 带主叫/被叫/坐席）
export type QcResult = {
  id: number
  call_id: string
  score: number
  hits: string // JSON 串，前端解析成 QcHit[]
  caller: string
  callee: string
  agent_ext: string
  created_at: string
}

// 提交转写 + 质检，有录音才可调。后台执行，拿 job_id 后轮询 getTranscribeJob。
export function transcribeCall(callId: string) {
  return request.post('/api/v1/cc/qc/transcribe', { call_id: callId }) as Promise<QcTranscribeAccepted>
}

// 查转写任务状态（done 时一次即拿到全文与质检结论）
export function getTranscribeJob(jobId: number) {
  return request.get(`/api/v1/cc/qc/transcribe/jobs/${jobId}`) as Promise<QcTranscribeResult>
}

// 按通话查最近一次转写任务（刷新页面后不必记 job id）
export function getTranscribeJobByCall(callId: string) {
  return request.get('/api/v1/cc/qc/transcribe/jobs', {
    params: { call_id: callId },
  }) as Promise<QcTranscribeResult>
}

// 单查某通话的转写全文
export function getTranscript(callId: string) {
  return request.get('/api/v1/cc/qc/transcripts', { params: { call_id: callId } }) as Promise<QcTranscript>
}

// 质检结果列表（分页）
export function listQcResults(params: { page: number; page_size: number }) {
  return request.get('/api/v1/cc/qc/results', { params }) as Promise<{
    list: QcResult[]; total: number; page: number; page_size: number
  }>
}

export function listQcKeywords() {
  return request.get('/api/v1/cc/qc/keywords') as Promise<FsRowsResult<QcKeyword>>
}

export function createQcKeyword(data: { word: string; type: 'forbidden' | 'required'; remark?: string }) {
  return request.post('/api/v1/cc/qc/keywords', data) as Promise<QcKeyword>
}

export function deleteQcKeyword(id: number) {
  return request.delete(`/api/v1/cc/qc/keywords/${id}`) as Promise<{ ok: boolean }>
}
