import dayjs from 'dayjs'
import i18n from '@/i18n/init'

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
  if (d) parts.push(i18n.t('{{d}}天', { d }))
  if (h) parts.push(i18n.t('{{h}}小时', { h }))
  if (m || !parts.length) parts.push(i18n.t('{{m}}分', { m }))
  return parts.join(' ')
}
