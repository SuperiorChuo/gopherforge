import dayjs from 'dayjs'

/** 把后端 RFC3339 时间格式化为 YYYY-MM-DD HH:mm:ss，空值显示 '-' */
export const formatDateTime = (value?: string | null) =>
  value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '-'

/** 字节数转可读大小 */
export const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`
}

/** 秒数转「N天 N小时 N分」 */
export const formatDuration = (seconds: number): string => {
  if (!seconds || seconds <= 0) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const parts: string[] = []
  if (d) parts.push(`${d}天`)
  if (h) parts.push(`${h}小时`)
  if (m || !parts.length) parts.push(`${m}分`)
  return parts.join(' ')
}
