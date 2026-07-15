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
