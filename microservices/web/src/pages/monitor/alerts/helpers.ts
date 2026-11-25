import type dayjs from 'dayjs'
import type {
  AlertEventListParams,
  AlertRuleListParams,
  AlertRulePayload,
  AlertRuleState,
  AlertSeverity,
} from '@/api/monitor'
import type { StatusTone } from '@/components/common/StatusPill'

export type ViewMode = 'rules' | 'events'

// 表单里 silence_until 由 DatePicker 持有为 dayjs，提交时再转 ISO 字符串
export type AlertRuleFormValues = Omit<AlertRulePayload, 'silence_until'> & {
  silence_until: dayjs.Dayjs | null
}

export type AlertRuleFilterValues = Omit<AlertRuleListParams, 'page' | 'page_size'>

export type AlertEventFilterValues = Omit<AlertEventListParams, 'page' | 'page_size' | 'rule_id'>

export const OPERATOR_LABELS: Record<string, string> = {
  gt: '大于',
  gte: '大于等于',
  lt: '小于',
  lte: '小于等于',
}

export const STATE_META: Record<AlertRuleState, { label: string; tone: StatusTone }> = {
  ok: { label: '正常', tone: 'success' },
  pending: { label: '等待确认', tone: 'warning' },
  firing: { label: '告警中', tone: 'danger' },
  error: { label: '采集异常', tone: 'danger' },
}

export const SEVERITY_META: Record<AlertSeverity, { label: string; color: string }> = {
  info: { label: '提示', color: 'blue' },
  warning: { label: '警告', color: 'gold' },
  critical: { label: '严重', color: 'red' },
}

export const NOTIFY_META: Record<string, { label: string; color?: string }> = {
  pending: { label: '待发送' },
  sent: { label: '已发送', color: 'green' },
  skipped: { label: '已跳过' },
  failed: { label: '发送失败', color: 'red' },
}

export const defaultRuleParams: AlertRuleListParams = { page: 1, page_size: 10 }
export const defaultEventParams: AlertEventListParams = { page: 1, page_size: 10 }

export function metricUnitLabel(unit: string | undefined, t: (s: string) => string) {
  if (unit === 'percent') return '%'
  if (unit === 'bytes') return 'Bytes'
  if (unit === 'count') return t('个')
  return unit || '-'
}

export function formatMetricValue(value: number, unit?: string) {
  if (unit === 'percent') return `${value.toFixed(2)}%`
  if (unit === 'bytes') return formatBytes(value)
  if (unit === 'count') return Math.round(value).toLocaleString('zh-CN')
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 1024) return `${Math.round(value)} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let unit = -1
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size.toFixed(2)} ${units[unit]}`
}
