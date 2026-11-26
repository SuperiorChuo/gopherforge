// 3D 办公室场景纯几何与布局计算辅助函数（无 React 组件，便于复用与静态分析）。
import * as THREE from 'three'
import type { SeatStatus } from '@/api/ai/hermes-office'
import { seatStatus } from '@/api/ai/hermes-office'
import type { DepartmentPodLayout } from '../officeLayout'
import type { SeatDraft } from '../useSeatEditor'

export interface PodStatusSegment {
  status: SeatStatus
  count: number
  width: number
  x: number
}

export interface CameraFraming {
  extent: number
  zoom: number
  far: number
}

export const POD_STATUS_ORDER: readonly SeatStatus[] = ['online', 'error', 'unprovisioned', 'offline', 'empty']

export const DEFAULT_PULSE_RING_ARGS: [number, number, number] = [1.03, 1.18, 48]
export const UNPROVISIONED_PULSE_RING_ARGS: [number, number, number] = [1.07, 1.16, 48]

export function createDropShadowTexture(color: string): THREE.CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 256
  const context = canvas.getContext('2d')
  if (context) {
    const gradient = context.createRadialGradient(128, 128, 12, 128, 128, 128)
    gradient.addColorStop(0, color)
    gradient.addColorStop(0.64, color.replace(/[\d.]+\)$/, '0.1)'))
    gradient.addColorStop(1, color.replace(/[\d.]+\)$/, '0)'))
    context.fillStyle = gradient
    context.fillRect(0, 0, 256, 256)
  }
  return new THREE.CanvasTexture(canvas)
}

export function computeFloorTiles(width: number, depth: number): Array<[number, number]> {
  const result: Array<[number, number]> = []
  if (width * depth > 4000) return result
  for (let x = 0; x < Math.floor(width); x += 1) {
    for (let z = 0; z < Math.floor(depth); z += 1) {
      if ((x * 7 + z * 11) % 9 === 0) result.push([x + 0.5, z + 0.5])
      if (result.length >= 300) return result
    }
  }
  return result
}

export function computeCirculationLanes(pods: DepartmentPodLayout[]): { vertical: number[]; horizontal: number[] } {
  const centers = (values: number[]) => {
    const unique = [...new Set(values.map((value) => Number(value.toFixed(3))))].sort((a, b) => a - b)
    return unique.slice(1).map((value, index) => (unique[index] + value) / 2)
  }
  return {
    vertical: centers(pods.map((pod) => pod.x)),
    horizontal: centers(pods.map((pod) => pod.z)),
  }
}

export function computePodStatusCounts(
  seatIds: number[],
  seats: Map<number, SeatDraft>,
): Record<SeatStatus, number> {
  return seatIds.reduce<Record<SeatStatus, number>>(
    (counts, seatId) => {
      const view = seats.get(seatId)
      if (view) counts[seatStatus(view)] += 1
      return counts
    },
    { online: 0, offline: 0, error: 0, unprovisioned: 0, empty: 0 },
  )
}

export function computePodStatusSegments(width: number, counts: Record<SeatStatus, number>): {
  stripWidth: number
  segments: PodStatusSegment[]
} {
  const active = POD_STATUS_ORDER.map((status) => ({ status, count: counts[status] })).filter(
    (segment) => segment.count > 0,
  )
  const total = Math.max(1, active.reduce((sum, segment) => sum + segment.count, 0))
  const stripWidth = Math.max(0.8, width - 0.72)
  const gap = 0.045
  const contentWidth = stripWidth - Math.max(0, active.length - 1) * gap
  let cursor = -stripWidth / 2
  const segments = active.map((segment) => {
    const segmentWidth = contentWidth * (segment.count / total)
    const x = cursor + segmentWidth / 2
    cursor += segmentWidth + gap
    return { ...segment, width: segmentWidth, x }
  })
  return { stripWidth, segments }
}

export function computeCameraTarget(
  center: [number, number, number],
  editing: boolean,
  pods: DepartmentPodLayout[],
): [number, number, number] {
  if (editing || pods.length === 0) return center
  return [
    pods.reduce((sum, pod) => sum + pod.x, 0) / pods.length,
    0.5,
    pods.reduce((sum, pod) => sum + pod.z, 0) / pods.length,
  ]
}

export function computeCameraFraming(
  width: number,
  depth: number,
  viewportWidth: number,
  viewportHeight: number,
  framingPadding: number,
): CameraFraming {
  const extent = Math.max(width, depth)
  const projectedWidth = (width + depth) * 0.72
  const projectedHeight = extent * 0.82
  const responsivePadding = viewportWidth < 560 ? framingPadding * 1.18 : framingPadding
  const minimumZoom = viewportWidth < 560 ? 6.25 : 9
  const zoom = Math.max(
    minimumZoom,
    Math.min(
      80,
      Math.min(viewportWidth / (projectedWidth + 4.5), viewportHeight / (projectedHeight + 4.5)) / responsivePadding,
    ),
  )
  return {
    extent,
    zoom,
    far: Math.max(240, extent * 8),
  }
}
