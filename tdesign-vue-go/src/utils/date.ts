// 获取常用时间
import dayjs from 'dayjs';

export const LAST_7_DAYS = [
  dayjs().subtract(7, 'day').format('YYYY-MM-DD'),
  dayjs().subtract(1, 'day').format('YYYY-MM-DD'),
];

export const LAST_30_DAYS = [
  dayjs().subtract(30, 'day').format('YYYY-MM-DD'),
  dayjs().subtract(1, 'day').format('YYYY-MM-DD'),
];

/**
 * 格式化日期时间
 * @param dateStr 日期字符串
 * @param format 格式，默认 'YYYY-MM-DD HH:mm:ss'
 */
export function formatDateTime(dateStr?: string | null, format = 'YYYY-MM-DD HH:mm:ss'): string {
  if (!dateStr) return '-';
  const date = dayjs(dateStr);
  return date.isValid() ? date.format(format) : '-';
}

/**
 * 格式化日期（不含时间）
 * @param dateStr 日期字符串
 */
export function formatDateOnly(dateStr?: string | null): string {
  return formatDateTime(dateStr, 'YYYY-MM-DD');
}
