// Hermes 办公室共享的非 3D 常量、数据类型与纯函数。
// 严禁引入 three 或 @react-three/*，确保页面入口与 Scene 3D chunk 彻底解耦。
import type { SeatStatus } from '@/api/ai/hermes-office'
import type { StatusTone } from '@/components/common/StatusPill'
import type { Department } from '@/types'

export const SEAT_STATUS_COLORS: Record<SeatStatus, string> = {
  online: '#22b96f',
  offline: '#94a3b8',
  error: '#ef5350',
  unprovisioned: '#eaa229',
  empty: '#c4ccd7',
}

export interface OfficeDepartmentPod {
  key: string
  name: string
  color: string
  seatIds: number[]
}

export const STATUS_META: Record<SeatStatus, { label: string; tone: StatusTone }> = {
  online: { label: '在线', tone: 'success' },
  offline: { label: '离线', tone: 'muted' },
  error: { label: '异常', tone: 'danger' },
  unprovisioned: { label: '未开通', tone: 'warning' },
  empty: { label: '空位', tone: 'muted' },
}

// 部门分类色板（dataviz 校验通过的 8 槽，明暗各一组；按全局部门顺序固定分配，不循环）。
// light 组里 aqua/yellow/magenta 三槽对浅底不足 3:1，因此色点永远和文字标签成对出现。
export const DEPT_COLORS: Record<'light' | 'dark', string[]> = {
  light: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#4a3aa7', '#e34948'],
  dark: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'],
}

export const DEPT_FALLBACK: Record<'light' | 'dark', string> = {
  light: '#8b93a7',
  dark: '#6b7280',
}

export interface DeptStat {
  deptId: number
  name: string
  color: string
  count: number
}

export interface RiskSeatItem {
  id: number
  name: string
  dept: string
  podKey: string
  status: SeatStatus
  statusLabel: string
  color: string
}

export interface RiskAreaItem {
  key: string
  name: string
  color: string
  count: number
  errors: number
}

export interface StatTileItem {
  key: string
  label: string
  value: number
  dot: string | null
}

export interface FloorFormValues {
  name: string
  width: number
  depth: number
  sort?: number
}

export function flattenDepts(list: Department[], out: Department[] = []): Department[] {
  list.forEach((d) => {
    out.push(d)
    if (d.children?.length) flattenDepts(d.children, out)
  })
  return out
}
