// 浏览态部门展示岛：部门地台、状态分段条、风险信标、工位排列与部门悬浮标签。
import { Html, RoundedBox } from '@react-three/drei'
import { memo, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as THREE from 'three'
import type { SeatStatus } from '@/api/ai/hermes-office'
import { SEAT_STATUS_COLORS } from '../officeLayout'
import type { DepartmentPodLayout } from '../officeLayout'
import type { SeatDraft } from '../useSeatEditor'
import { computePodStatusCounts, computePodStatusSegments } from './geometry'
import { SeatMesh } from './SeatMesh'
import type { RoomTheme } from './theme'

const NOOP_DRAG_START = () => undefined

const PodStatusStrip = memo(function PodStatusStrip({
  width,
  depth,
  counts,
  room,
}: {
  width: number
  depth: number
  counts: Record<SeatStatus, number>
  room: RoomTheme
}) {
  const { stripWidth, segments } = useMemo(() => computePodStatusSegments(width, counts), [counts, width])

  return (
    <group position={[0, 0.335, depth / 2 - 0.2]}>
      <mesh>
        <boxGeometry args={[stripWidth + 0.14, 0.026, 0.13]} />
        <meshStandardMaterial color={room.floorSide} roughness={0.82} />
      </mesh>
      {segments.map((segment) => (
        <mesh key={segment.status} position={[segment.x, 0.018, 0]}>
          <boxGeometry args={[Math.max(0.035, segment.width), 0.035, 0.09]} />
          <meshStandardMaterial
            color={SEAT_STATUS_COLORS[segment.status]}
            emissive={SEAT_STATUS_COLORS[segment.status]}
            emissiveIntensity={segment.status === 'error' ? 0.28 : 0.06}
            roughness={0.58}
          />
        </mesh>
      ))}
    </group>
  )
})

export interface DepartmentPodProps {
  layout: DepartmentPodLayout
  seats: Map<number, SeatDraft>
  accents: Map<number, string>
  selectedSeatId: number | null
  compactLabels: boolean
  room: RoomTheme
  onSelectSeat: (seatId: number | null) => void
}

export const DepartmentPod = memo(function DepartmentPod({
  layout,
  seats,
  accents,
  selectedSeatId,
  compactLabels,
  room,
  onSelectSeat,
}: DepartmentPodProps) {
  const { t } = useTranslation()
  const statusCounts = useMemo(() => computePodStatusCounts(layout.seatIds, seats), [layout.seatIds, seats])
  const riskCount = statusCounts.error + statusCounts.unprovisioned
  const edgeColor = statusCounts.error
    ? SEAT_STATUS_COLORS.error
    : statusCounts.unprovisioned
      ? SEAT_STATUS_COLORS.unprovisioned
      : layout.color
  const selectedInPod = selectedSeatId != null && layout.seatIds.includes(selectedSeatId)
  const podTint = useMemo(
    () => new THREE.Color(layout.color).lerp(new THREE.Color(room.podTop), 0.64).getStyle(),
    [layout.color, room.podTop],
  )
  const edgeTint = useMemo(
    () => new THREE.Color(edgeColor).lerp(new THREE.Color(room.podTop), riskCount ? 0.22 : 0.1).getStyle(),
    [edgeColor, riskCount, room.podTop],
  )

  return (
    <group position={[layout.x, 0, layout.z]}>
      <RoundedBox
        args={[layout.width + 0.2, 0.18, layout.depth + 0.2]}
        radius={0.16}
        smoothness={6}
        position={[0, 0.09, 0]}
        receiveShadow
        castShadow
      >
        <meshStandardMaterial color={podTint} roughness={0.82} />
      </RoundedBox>
      <RoundedBox
        args={[layout.width, 0.18, layout.depth]}
        radius={0.15}
        smoothness={6}
        position={[0, 0.2, 0]}
        receiveShadow
        castShadow
      >
        <meshStandardMaterial
          color={room.podTop}
          emissive={selectedInPod ? room.ring : room.podTop}
          emissiveIntensity={selectedInPod ? 0.055 : 0}
          roughness={0.88}
        />
      </RoundedBox>
      <mesh position={[0, 0.31, -layout.depth / 2 + 0.035]}>
        <boxGeometry args={[layout.width - 0.12, 0.035, 0.07]} />
        <meshStandardMaterial color={edgeTint} emissive={edgeColor} emissiveIntensity={riskCount ? 0.12 : 0.02} />
      </mesh>
      <mesh position={[0, 0.31, layout.depth / 2 - 0.035]}>
        <boxGeometry args={[layout.width - 0.12, 0.035, 0.07]} />
        <meshStandardMaterial color={edgeTint} emissive={edgeColor} emissiveIntensity={riskCount ? 0.12 : 0.02} />
      </mesh>
      <mesh position={[-layout.width / 2 + 0.035, 0.31, 0]}>
        <boxGeometry args={[0.07, 0.035, layout.depth - 0.12]} />
        <meshStandardMaterial color={edgeTint} emissive={edgeColor} emissiveIntensity={riskCount ? 0.12 : 0.02} />
      </mesh>
      <mesh position={[layout.width / 2 - 0.035, 0.31, 0]}>
        <boxGeometry args={[0.07, 0.035, layout.depth - 0.12]} />
        <meshStandardMaterial color={edgeTint} emissive={edgeColor} emissiveIntensity={riskCount ? 0.12 : 0.02} />
      </mesh>

      <PodStatusStrip width={layout.width} depth={layout.depth} counts={statusCounts} room={room} />

      {riskCount > 0 && (
        <group position={[layout.width / 2 - 0.28, 0.5, -layout.depth / 2 + 0.28]}>
          <mesh>
            <cylinderGeometry args={[0.055, 0.055, 0.38, 16]} />
            {/* toneMapped=false 让发光柱在浅色场景里保持亮色（配合 Bloom 越过阈值） */}
            <meshStandardMaterial color={edgeColor} emissive={edgeColor} emissiveIntensity={1.1} toneMapped={false} />
          </mesh>
          <mesh position={[0, -0.2, 0]}>
            <cylinderGeometry args={[0.09, 0.09, 0.05, 16]} />
            <meshStandardMaterial color={room.metal} roughness={0.7} />
          </mesh>
        </group>
      )}

      {selectedInPod && (
        <RoundedBox
          args={[layout.width + 0.48, 0.055, layout.depth + 0.48]}
          radius={0.19}
          smoothness={6}
          position={[0, 0.08, 0]}
        >
          <meshBasicMaterial color={room.ring} transparent opacity={0.48} depthWrite={false} />
        </RoundedBox>
      )}

      {layout.seats.map((position) => {
        const view = seats.get(position.seatId)
        if (!view) return null
        return (
          <group key={position.seatId} position={[0, 0.29, 0]}>
            <SeatMesh
              view={view}
              accent={accents.get(position.seatId) ?? layout.color}
              selected={position.seatId === selectedSeatId}
              editing={false}
              room={room}
              compactLabel={compactLabels}
              displayX={position.x}
              displayZ={position.z}
              displayRotation={0}
              onSelect={onSelectSeat}
              onDragStart={NOOP_DRAG_START}
            />
          </group>
        )
      })}

      <Html position={[0, 0.46, layout.depth / 2 - 0.28]} center occlude={false}>
        <div
          className={`hermes-department-label${riskCount ? ' has-risk' : ''}${selectedInPod ? ' is-selected' : ''}`}
          style={{ color: room.podLabel }}
        >
          <span className="hermes-department-label-dot" style={{ background: layout.color }} />
          <span>{layout.name}</span>
          <span className="hermes-department-label-count">
            {t('{{a}}/{{b}} 在线', { a: statusCounts.online, b: layout.seatIds.length })}
          </span>
          {riskCount > 0 && <span className="hermes-department-label-risk">{t('{{n}} 风险', { n: riskCount })}</span>}
        </div>
      </Html>
    </group>
  )
})
