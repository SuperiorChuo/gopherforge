import dayjs, { type Dayjs } from 'dayjs'
import type { BpmFormSchema } from '@/api/bpm'

/** 快照（分/字符串日期）→ 表单初值（元/dayjs） */
export function snapshotToFormValues(
  schema: BpmFormSchema | null | undefined,
  snapshot?: Record<string, unknown> | null,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema?.fields ?? []) {
    const v = snapshot?.[f.key]
    if (v === undefined || v === null) continue
    if (f.type === 'amount' && typeof v === 'number') out[f.key] = v / 100
    else if (f.type === 'date' && typeof v === 'string' && v) out[f.key] = dayjs(v)
    else out[f.key] = v
  }
  return out
}

/** 表单值（元/dayjs）→ 快照（分/字符串日期）；空值剔除 */
export function formValuesToSnapshot(
  schema: BpmFormSchema | null | undefined,
  values: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema?.fields ?? []) {
    const v = values[f.key]
    if (v === undefined || v === null || v === '') continue
    if (f.type === 'amount' && typeof v === 'number') out[f.key] = Math.round(v * 100)
    else if (f.type === 'date') out[f.key] = (v as Dayjs).format('YYYY-MM-DD')
    else out[f.key] = v
  }
  return out
}
