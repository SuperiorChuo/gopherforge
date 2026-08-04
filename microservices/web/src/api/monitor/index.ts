import request from '@/utils/request'
import type { PageRequest, PageResponse, ScheduledJob } from '@/types'

type JobListParams = PageRequest & { name?: string; status?: number }
type JobCreateData = Omit<ScheduledJob, 'id' | 'created_at' | 'last_run_time' | 'next_run_time'>
type JobUpdateData = Partial<JobCreateData>

export interface ServerCPUInfo {
  model_name: string
  cores: number
  used_percent: number
}

export interface ServerStorageInfo {
  total: number
  used: number
  free: number
  used_percent: number
}

export interface ServerOSInfo {
  go_os: string
  arch: string
  compiler: string
  go_version: string
  num_goroutine: number
  hostname: string
  platform: string
  boot_time: string
}

export interface ServerInfo {
  cpu: ServerCPUInfo
  memory: ServerStorageInfo
  disk: ServerStorageInfo
  os: ServerOSInfo
  runtime?: ServerOSInfo
}

export const getServerInfo = () =>
  request.get<unknown, ServerInfo>('/api/v1/monitor/server')

export const getMySQLInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/mysql')

export const getRedisInfo = () =>
  request.get<unknown, Record<string, unknown>>('/api/v1/monitor/redis')

// 指标历史趋势：metric 与告警规则引擎同源（见 AlertMetricDefinition），
// range 支持 1h / 24h / 7d，后端按桶降采样
export interface TrendPoint {
  t: number
  value: number
}

export interface MetricTrend {
  metric: string
  range: '1h' | '24h' | '7d'
  unit: string
  points: TrendPoint[]
}

export type TrendRange = '1h' | '24h' | '7d'

export const TREND_RANGES: { value: TrendRange; label: string }[] = [
  { value: '1h', label: '1 小时' },
  { value: '24h', label: '24 小时' },
  { value: '7d', label: '7 天' },
]

export const getMetricTrends = (metric: string, range: TrendRange) =>
  request.get<unknown, MetricTrend>(`/api/v1/monitor/metrics/trends`, { params: { metric, range } })

export type AlertOperator = 'gt' | 'gte' | 'lt' | 'lte'
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type AlertRuleState = 'ok' | 'pending' | 'firing' | 'error'
export type AlertEventStatus = 'firing' | 'resolved'
export type AlertNotifyStatus = 'pending' | 'sent' | 'skipped' | 'failed'
export type AlertChannel = 'email' | 'station' | 'wecom'

export const ALERT_CHANNELS: { value: AlertChannel; label: string }[] = [
  { value: 'email', label: '邮件' },
  { value: 'station', label: '站内信' },
  { value: 'wecom', label: '企业微信' },
]

export interface AlertMetricDefinition {
  key: string
  title: string
  unit: 'percent' | 'bytes' | 'count' | string
  description: string
  operators: AlertOperator[]
}

export interface MonitorAlertRule {
  id: number
  name: string
  metric: string
  operator: AlertOperator
  threshold: number
  duration_seconds: number
  severity: AlertSeverity
  enabled: boolean
  notify_on_resolve: boolean
  notify_channels: AlertChannel[]
  silence_until?: string | null
  state: AlertRuleState
  pending_since?: string | null
  firing_since?: string | null
  last_value?: number | null
  last_evaluated_at?: string | null
  last_error: string
  created_at: string
  updated_at: string
}

export interface MonitorAlertEvent {
  id: number
  rule_id?: number | null
  rule_name: string
  metric: string
  severity: AlertSeverity
  status: AlertEventStatus
  value: number
  threshold: number
  message: string
  notify_status: AlertNotifyStatus
  notify_error: string
  notified_at?: string | null
  created_at: string
}

export interface MonitorAlertSummary {
  total: number
  enabled: number
  firing: number
  pending: number
  error: number
  checked_at: string
}

export interface AlertRulePayload {
  name: string
  metric: string
  operator: AlertOperator
  threshold: number
  duration_seconds: number
  severity: AlertSeverity
  enabled: boolean
  notify_on_resolve: boolean
  notify_channels: AlertChannel[]
  silence_until?: string | null
}

export type AlertRuleListParams = PageRequest & {
  name?: string
  metric?: string
  state?: AlertRuleState
  enabled?: boolean
}

export type AlertEventListParams = PageRequest & {
  rule_id?: number
  rule_name?: string
  status?: AlertEventStatus
  severity?: AlertSeverity
  notify_status?: AlertNotifyStatus
}

export const getAlertMetrics = () =>
  request.get<unknown, { list: AlertMetricDefinition[] }>('/api/v1/monitor/alert-metrics')

export const getAlertSummary = () =>
  request.get<unknown, MonitorAlertSummary>('/api/v1/monitor/alert-summary')

export const getAlertRules = (params: AlertRuleListParams) =>
  request.get<unknown, PageResponse<MonitorAlertRule>>('/api/v1/monitor/alert-rules', { params })

export const createAlertRule = (data: AlertRulePayload) =>
  request.post<unknown, MonitorAlertRule>('/api/v1/monitor/alert-rules', data)

export const updateAlertRule = (id: number, data: AlertRulePayload) =>
  request.put<unknown, MonitorAlertRule>(`/api/v1/monitor/alert-rules/${id}`, data)

export const deleteAlertRule = (id: number) =>
  request.delete<unknown, void>(`/api/v1/monitor/alert-rules/${id}`)

export const evaluateAlertRule = (id: number) =>
  request.post<unknown, { rule: MonitorAlertRule; event?: MonitorAlertEvent | null }>(
    `/api/v1/monitor/alert-rules/${id}/evaluate`,
  )

export const getAlertEvents = (params: AlertEventListParams) =>
  request.get<unknown, PageResponse<MonitorAlertEvent>>('/api/v1/monitor/alert-events', { params })

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
