import request from '@/utils/request'
import type { PageRequest, PageResponse, ScheduledJob } from '@/types'

type JobListParams = PageRequest & { name?: string; status?: number }
type JobCreateData = Omit<ScheduledJob, 'id' | 'created_at' | 'last_run_time' | 'next_run_time'>
type JobUpdateData = Partial<JobCreateData>

export const getServerInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/server')

export const getMySQLInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/mysql')

export const getRedisInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/redis')

export const getJobList = (params: JobListParams) =>
  request.get<unknown, PageResponse<ScheduledJob>>('/api/v1/monitor/jobs', { params })

export interface JobHealth {
  total: number
  enabled: number
  paused: number
  recent_failed: number
  last_run_time?: string
  window_hours: number
}

export const getJobHealth = () =>
  request.get<unknown, JobHealth>('/api/v1/monitor/jobs/health')

// 分布式任务心跳（任务中心）：各服务进程内循环 + 主机 shell cron 的最近运行状态
export interface JobHeartbeat {
  id: number
  job_key: string
  service: string
  description: string
  interval_sec: number
  last_run_at: string
  last_status: 'ok' | 'error'
  last_error: string
  last_duration_ms: number
  runs: number
  fails: number
  // stale=超期未上报（超过 2 倍期望间隔），大概率任务已停摆
  stale: boolean
}

export const getJobHeartbeats = () =>
  request.get<unknown, { list: JobHeartbeat[]; total: number }>('/api/v1/monitor/jobs/heartbeats')

// 微服务健康总览：monitor 并发探测各服务 /health/ready
export interface ServiceHealthRow {
  name: string
  ok: boolean
  http_code: number
  latency_ms: number
  error?: string
}

export const getServicesHealth = () =>
  request.get<unknown, { list: ServiceHealthRow[]; total: number; healthy: number; checked_at: string }>(
    '/api/v1/monitor/services',
  )

export const createJob = (data: JobCreateData) =>
  request.post<unknown, ScheduledJob>('/api/v1/monitor/jobs', data)

export const updateJob = (id: number, data: JobUpdateData) =>
  request.put<unknown, ScheduledJob>(`/api/v1/monitor/jobs/${id}`, data)

export const deleteJob = (id: number) =>
  request.delete<unknown, void>(`/api/v1/monitor/jobs/${id}`)

export const startJob = (id: number) =>
  request.post<unknown, void>(`/api/v1/monitor/jobs/${id}/start`)

export const stopJob = (id: number) =>
  request.post<unknown, void>(`/api/v1/monitor/jobs/${id}/stop`)

export const runJob = (id: number) =>
  request.post<unknown, void>(`/api/v1/monitor/jobs/${id}/run`)

export const cleanupJobLogs = (retention_days: number) =>
  request.post<unknown, { deleted_rows: number }>('/api/v1/monitor/job-logs/cleanup', { retention_days })

// 可调度的内置目标。后端与执行分发共用同一张表，所以下拉里出现的目标一定
// 跑得起来；title/description 是英文（monitor 服务源码强制英文），中文标签
// 由下面的 JOB_TARGET_LABELS 兜，缺映射时回落显示 target 本身。
export interface JobTarget {
  target: string
  title: string
  description: string
}

export const getJobTargets = () =>
  request.get<unknown, { list: JobTarget[] }>('/api/v1/monitor/jobs/targets')

export const JOB_TARGET_LABELS: Record<string, { label: string; hint: string }> = {
  CleanExpiredLogs: { label: '清理过期任务日志', hint: '删除 30 天前的调度执行日志' },
  HealthCheck: { label: '调度健康巡检', hint: '统计近 24 小时的任务运行与失败情况' },
}

export interface ScheduledJobLog {
  id: number
  job_id: number
  job_name: string
  // 1=成功 0=失败
  status: number
  message: string
  // 毫秒
  duration: number
  created_at: string
}

type JobLogListParams = PageRequest & { job_id?: number; status?: number }

export const getJobLogList = (params: JobLogListParams) =>
  request.get<unknown, PageResponse<ScheduledJobLog>>('/api/v1/monitor/job-logs', { params })
