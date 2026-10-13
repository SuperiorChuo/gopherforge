// Hermes 办公室布局算法与共享视觉常量。3D 场景的部门岛与工位排列完全由此生成。
import type { SeatStatus } from '@/api/ai/hermes-office'

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

export interface DepartmentPodLayout extends OfficeDepartmentPod {
  x: number
  z: number
  width: number
  depth: number
  seats: Array<{ seatId: number; x: number; z: number }>
}

export interface OverviewLayout {
  width: number
  depth: number
  pods: DepartmentPodLayout[]
}

export function buildOverviewLayout(pods: OfficeDepartmentPod[]): OverviewLayout {
  const specs = pods
    .filter((pod) => pod.seatIds.length > 0)
    .map((pod) => {
      const columns = Math.min(4, Math.max(1, Math.ceil(Math.sqrt(pod.seatIds.length * 1.25))))
      const rows = Math.ceil(pod.seatIds.length / columns)
      const width = (columns - 1) * 2.35 + 4.1
      const depth = (rows - 1) * 2.15 + 4.1
      const podSeats = pod.seatIds.map((seatId, index) => ({
        seatId,
        x: (index % columns - (columns - 1) / 2) * 2.35,
        z: (Math.floor(index / columns) - (rows - 1) / 2) * 2.15 - 0.35,
      }))
      return { ...pod, width, depth, seats: podSeats }
    })

  if (specs.length === 0) return { width: 10, depth: 8, pods: [] }

  const columns = Math.max(1, Math.ceil(Math.sqrt(specs.length * 1.35)))
  const rows = Math.ceil(specs.length / columns)
  const cellWidth = Math.max(...specs.map((pod) => pod.width))
  const cellDepth = Math.max(...specs.map((pod) => pod.depth))
  const gap = 3
  const contentWidth = columns * cellWidth + (columns - 1) * gap
  const contentDepth = rows * cellDepth + (rows - 1) * gap
  const singlePod = specs.length === 1
  const width = Math.max(singlePod ? 6.4 : 8.5, contentWidth + (singlePod ? 2.2 : 4.2))
  const depth = Math.max(singlePod ? 6 : 8, contentDepth + (singlePod ? 2.1 : 4))
  const originX = (width - contentWidth) / 2 + cellWidth / 2
  const focusOffset = singlePod ? Math.min(0.45, depth * 0.06) : 0
  const originZ = (depth - contentDepth) / 2 + cellDepth / 2 + focusOffset
  const slots = Array.from({ length: columns * rows }, (_, index) => {
    const column = index % columns
    const row = Math.floor(index / columns)
    return {
      column,
      row,
      x: originX + column * (cellWidth + gap),
      z: originZ + row * (cellDepth + gap),
      distance: Math.hypot(column - (columns - 1) / 2, row - (rows - 1) / 2),
    }
  }).sort((a, b) => a.distance - b.distance || a.row - b.row || a.column - b.column)

  return {
    width,
    depth,
    // Pods arrive largest-first. Assigning them to center-out slots keeps the busiest teams in the visual focus.
    pods: specs.map((pod, index) => ({ ...pod, x: slots[index].x, z: slots[index].z })),
  }
}
