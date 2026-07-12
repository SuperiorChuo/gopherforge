import request from '@/utils/request'
import type { PageRequest, PageResponse, ScheduledJob } from '@/types'

type JobListParams = PageRequest & { keyword?: string; status?: number }
type JobCreateData = Omit<ScheduledJob, 'id' | 'created_at'>
type JobUpdateData = Partial<JobCreateData>
type JobLogListParams = PageRequest & { job_id?: number; start_time?: string; end_time?: string }

export const getServerInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/server')

export const getMySQLInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/mysql')

export const getRedisInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/redis')

export const getJobList = (params: JobListParams) =>
  request.get<unknown, PageResponse<ScheduledJob>>('/api/v1/monitor/jobs', { params })

export const createJob = (data: JobCreateData) =>
  request.post<unknown, ScheduledJob>('/api/v1/monitor/jobs', data)

export const updateJob = (id: number, data: JobUpdateData) =>
  request.put<unknown, ScheduledJob>(`/api/v1/monitor/jobs/${id}`, data)

export const deleteJob = (id: number) =>
  request.delete<unknown, void>(`/api/v1/monitor/jobs/${id}`)

export const startJob = (id: number) =>
  request.put<unknown, void>(`/api/v1/monitor/jobs/${id}/start`)

export const stopJob = (id: number) =>
  request.put<unknown, void>(`/api/v1/monitor/jobs/${id}/stop`)

export const runJob = (id: number) =>
  request.post<unknown, void>(`/api/v1/monitor/jobs/${id}/run`)

export const getJobLogs = (params: JobLogListParams) =>
  request.get<unknown, PageResponse<Record<string, unknown>>>('/api/v1/monitor/job-logs', { params })

export const cleanupJobLogs = (retention_days: number) =>
  request.post<unknown, { deleted_rows: number }>('/api/v1/monitor/job-logs/cleanup', { retention_days })
