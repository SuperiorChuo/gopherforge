import request from '@/utils/request'
import type { HermesActualState, HermesHealthStatus } from './hermes'

export interface HermesOfficeFloor {
  id: number
  tenant_id: number
  name: string
  sort: number
  /** 平面尺寸（米），3D 里 width→X 轴、depth→Z 轴 */
  width: number
  depth: number
  created_at: string
  updated_at: string
}

export interface HermesOfficeSeat {
  id: number
  tenant_id: number
  floor_id: number
  label: string
  x: number
  /** 平面第二轴，3D 里映射到 z */
  y: number
  /** 朝向（度） */
  rotation: number
  /** 0 = 空位 */
  user_id: number
}

/** 工位 + 绑定员工 + 该员工 Hermes 设备状态。后端一次 JOIN 返回，前端不再逐个查。 */
export interface HermesOfficeSeatView {
  seat: HermesOfficeSeat
  user: { id: number; username: string; nickname: string } | null
  device: {
    device_id: string
    actual_state: HermesActualState
    health_status: HermesHealthStatus
    hermes_version: string
    last_seen_at?: string
  } | null
}

/** 工位在地图上的呈现状态，由 seat/user/device 三者推导 */
export type SeatStatus = 'online' | 'offline' | 'error' | 'unprovisioned' | 'empty'

export function seatStatus(view: HermesOfficeSeatView): SeatStatus {
  if (!view.user) return 'empty'
  if (!view.device) return 'unprovisioned'
  const { actual_state, health_status } = view.device
  if (actual_state === 'error' || actual_state === 'revoked') return 'error'
  if (health_status === 'unhealthy' || health_status === 'policy_drift') return 'error'
  if (actual_state === 'active') return 'online'
  return 'offline'
}

export const listOfficeFloors = () =>
  request.get<unknown, HermesOfficeFloor[]>('/api/v1/ai/hermes/office/floors')

export const createOfficeFloor = (data: { name: string; width: number; depth: number; sort?: number }) =>
  request.post<unknown, HermesOfficeFloor>('/api/v1/ai/hermes/office/floors', data)

export const updateOfficeFloor = (
  id: number,
  data: { name: string; width: number; depth: number; sort?: number },
) => request.put<unknown, HermesOfficeFloor>(`/api/v1/ai/hermes/office/floors/${id}`, data)

export const deleteOfficeFloor = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/office/floors/${id}`)

export const listOfficeSeats = (floorId: number) =>
  request.get<unknown, HermesOfficeSeatView[]>(`/api/v1/ai/hermes/office/floors/${floorId}/seats`)

/** 拖拽编辑器整层一次性提交，避免逐个工位打请求 */
export const bulkUpsertOfficeSeats = (
  floorId: number,
  seats: Array<Pick<HermesOfficeSeat, 'label' | 'x' | 'y' | 'rotation' | 'user_id'> & { id?: number }>,
) => request.put<unknown, void>(`/api/v1/ai/hermes/office/floors/${floorId}/seats`, { seats })

export const deleteOfficeSeat = (seatId: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/office/seats/${seatId}`)
