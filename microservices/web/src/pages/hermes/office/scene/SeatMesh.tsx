// 单个工位 3D 渲染单元：交互热区、底垫、状态提示环、办公桌/显示器/座椅、在岗人物与悬浮信息牌。
import { Html, RoundedBox } from '@react-three/drei'
import { memo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { HermesOfficeSeatView, SeatStatus } from '@/api/ai/hermes-office'
import { seatStatus } from '@/api/ai/hermes-office'
import { formatDateTime } from '@/utils/format'
import { HEALTH_LABELS } from '../../_shared'
import { SEAT_STATUS_COLORS } from '../officeLayout'
import type { SeatDraft } from '../useSeatEditor'
import { UNPROVISIONED_PULSE_RING_ARGS } from './geometry'
import { Person, Plant } from './OfficeProps'
import { SelectionMarker, StatusPulseRing } from './StatusEffects'
import { SEAT_STATUS_LABELS } from './theme'
import type { RoomTheme } from './theme'

const DESK_PLANT_POS: [number, number, number] = [0.55, 0.87, -0.5]

const SeatWorkstation = memo(function SeatWorkstation({
  accent,
  room,
  status,
  statusColor,
  hasDeskPlant,
  showPerson,
}: {
  accent?: string
  room: RoomTheme
  status: SeatStatus
  statusColor: string
  hasDeskPlant: boolean
  showPerson: boolean
}) {
  return (
    <>
      <RoundedBox args={[1.5, 0.07, 0.7]} radius={0.025} smoothness={4} position={[0, 0.72, -0.35]} castShadow>
        <meshStandardMaterial color={room.deskTop} roughness={0.48} />
      </RoundedBox>
      {/* 细件不投射阴影：桌腿/背板/显示器底座/支柱/椅背在工位尺度下阴影不可辨，
          省掉每工位约 7 个 caster——mat/桌面/显示器/人体已给出占位轮廓 */}
      <mesh position={[-0.71, 0.36, -0.35]}>
        <boxGeometry args={[0.06, 0.72, 0.64]} />
        <meshStandardMaterial color={room.deskTop} roughness={0.6} />
      </mesh>
      <mesh position={[0.71, 0.36, -0.35]}>
        <boxGeometry args={[0.06, 0.72, 0.64]} />
        <meshStandardMaterial color={room.deskTop} roughness={0.6} />
      </mesh>
      <mesh position={[0, 0.42, -0.64]}>
        <boxGeometry args={[1.38, 0.36, 0.04]} />
        <meshStandardMaterial color={room.deskLeg} roughness={0.72} />
      </mesh>

      <mesh position={[0, 0.8, -0.55]}>
        <boxGeometry args={[0.1, 0.15, 0.07]} />
        <meshStandardMaterial color={room.screenFrame} roughness={0.5} />
      </mesh>
      <RoundedBox args={[0.68, 0.42, 0.05]} radius={0.018} smoothness={4} position={[0, 1.06, -0.56]} castShadow>
        <meshStandardMaterial color={room.screenFrame} roughness={0.45} />
      </RoundedBox>
      <mesh position={[0, 1.06, -0.53]}>
        <planeGeometry args={[0.6, 0.34]} />
        <meshStandardMaterial
          color={status === 'online' ? '#c7f0dc' : '#d2d8e0'}
          emissive={statusColor}
          // online/error 屏幕 toneMapped=false 且提亮到越过 Bloom 阈值（0.9），
          // 读出"亮屏光晕"；unprovisioned/offline/empty 保持在阈值下不发光
          emissiveIntensity={
            status === 'online' ? 1.1 : status === 'error' ? 1.0 : status === 'unprovisioned' ? 0.22 : status === 'offline' ? 0.14 : 0.08
          }
          toneMapped={false}
          roughness={0.32}
        />
      </mesh>
      <mesh position={[-0.18, 1.11, -0.505]}>
        <planeGeometry args={[0.2, 0.028]} />
        <meshBasicMaterial color={room.screenLine} />
      </mesh>
      <mesh position={[-0.12, 1.04, -0.505]}>
        <planeGeometry args={[0.32, 0.025]} />
        <meshBasicMaterial color={statusColor} transparent opacity={0.7} />
      </mesh>
      <mesh position={[-0.21, 0.98, -0.505]}>
        <planeGeometry args={[0.14, 0.02]} />
        <meshBasicMaterial color={room.screenLine} transparent opacity={0.72} />
      </mesh>

      <mesh position={[0, 0.765, -0.2]}>
        <boxGeometry args={[0.44, 0.022, 0.15]} />
        <meshStandardMaterial color={room.keyboard} roughness={0.62} />
      </mesh>
      <mesh position={[0.33, 0.762, -0.2]}>
        <boxGeometry args={[0.07, 0.02, 0.11]} />
        <meshStandardMaterial color={room.keyboard} roughness={0.62} />
      </mesh>
      <mesh position={[-0.55, 0.8, -0.32]}>
        <cylinderGeometry args={[0.045, 0.04, 0.1, 14]} />
        <meshStandardMaterial color={accent ?? room.chair} roughness={0.7} />
      </mesh>
      {hasDeskPlant && <Plant position={DESK_PLANT_POS} scale={0.5} room={room} />}
      <mesh position={[0.6, 0.83, -0.12]}>
        <cylinderGeometry args={[0.036, 0.036, 0.16, 16]} />
        <meshStandardMaterial color={statusColor} emissive={statusColor} emissiveIntensity={1.1} />
      </mesh>
      <mesh position={[0.6, 0.745, -0.12]}>
        <cylinderGeometry args={[0.055, 0.055, 0.025, 16]} />
        <meshStandardMaterial color={room.metal} roughness={0.62} />
      </mesh>

      <mesh position={[0, 0.02, 0.42]}>
        <cylinderGeometry args={[0.21, 0.21, 0.035, 20]} />
        <meshStandardMaterial color={room.chair} roughness={0.68} />
      </mesh>
      <mesh position={[0, 0.2, 0.42]}>
        <cylinderGeometry args={[0.04, 0.04, 0.32, 12]} />
        <meshStandardMaterial color={room.chair} roughness={0.68} />
      </mesh>
      <RoundedBox args={[0.48, 0.08, 0.46]} radius={0.03} smoothness={4} position={[0, 0.4, 0.42]}>
        <meshStandardMaterial color={room.chair} roughness={0.72} />
      </RoundedBox>
      <RoundedBox args={[0.46, 0.52, 0.07]} radius={0.03} smoothness={4} position={[0, 0.7, 0.64]}>
        <meshStandardMaterial color={room.chair} roughness={0.72} />
      </RoundedBox>
      <mesh position={[-0.3, 0.44, 0.43]}>
        <boxGeometry args={[0.08, 0.06, 0.34]} />
        <meshStandardMaterial color={room.metal} roughness={0.7} />
      </mesh>
      <mesh position={[0.3, 0.44, 0.43]}>
        <boxGeometry args={[0.08, 0.06, 0.34]} />
        <meshStandardMaterial color={room.metal} roughness={0.7} />
      </mesh>

      {showPerson && <Person accent={accent ?? SEAT_STATUS_COLORS.online} room={room} />}
    </>
  )
})

const SeatLabel = memo(function SeatLabel({
  label,
  status,
  statusColor,
  statusLabel,
  selected,
  hovered,
  device,
}: {
  label: string
  status: SeatStatus
  statusColor: string
  statusLabel: string
  selected: boolean
  hovered: boolean
  device: HermesOfficeSeatView['device']
}) {
  const { t } = useTranslation()

  return (
    <Html position={[0, 1.7, -0.35]} center occlude={false}>
      <div
        className={`hermes-seat-pill is-${status}${selected ? ' is-selected' : ''}${hovered ? ' is-hovered' : ''}`}
      >
        <span className="hermes-seat-pill-dot" style={{ background: statusColor }} />
        <span className="hermes-seat-pill-copy">
          <span className="hermes-seat-pill-row">
            <strong>{label}</strong>
            {(selected || hovered || status === 'error' || status === 'unprovisioned') && (
              <span className="hermes-seat-pill-sub">{t(statusLabel)}</span>
            )}
          </span>
          {device && (selected || hovered) && (
            <span className={`hermes-seat-pill-meta${HEALTH_LABELS[device.health_status].bad ? ' is-bad' : ''}`}>
              {device.hermes_version ? `v${device.hermes_version}` : '—'}
              {` · ${t(HEALTH_LABELS[device.health_status].label)}`}
              {device.last_seen_at ? ` · ${formatDateTime(device.last_seen_at)}` : ''}
            </span>
          )}
        </span>
      </div>
    </Html>
  )
})

export interface SeatMeshProps {
  view: SeatDraft
  accent?: string
  selected: boolean
  editing: boolean
  room: RoomTheme
  compactLabel?: boolean
  displayX?: number
  displayZ?: number
  displayRotation?: number
  onSelect: (seatId: number) => void
  onDragStart: (seatId: number) => void
}

export const SeatMesh = memo(function SeatMesh({
  view,
  accent,
  selected,
  editing,
  room,
  compactLabel = false,
  displayX,
  displayZ,
  displayRotation,
  onSelect,
  onDragStart,
}: SeatMeshProps) {
  const { seat } = view
  const [hovered, setHovered] = useState(false)
  const status = seatStatus(view)
  const statusColor = SEAT_STATUS_COLORS[status]
  const statusLabel = SEAT_STATUS_LABELS[status]
  const label = view.user ? view.user.nickname || view.user.username : seat.label || '空位'
  const hasDeskPlant = Math.abs(seat.id) % 3 === 0

  return (
    <group
      position={[displayX ?? seat.x, 0, displayZ ?? seat.y]}
      rotation={[0, ((displayRotation ?? seat.rotation) * Math.PI) / 180, 0]}
    >
      <mesh
        position={[0, 0.7, 0]}
        onPointerOver={(event) => {
          event.stopPropagation()
          setHovered(true)
        }}
        onPointerOut={() => setHovered(false)}
        onClick={(event) => {
          event.stopPropagation()
          onSelect(seat.id)
        }}
        onPointerDown={(event) => {
          if (!editing) return
          event.stopPropagation()
          onSelect(seat.id)
          onDragStart(seat.id)
        }}
      >
        <boxGeometry args={[2, 1.45, 1.8]} />
        <meshBasicMaterial transparent opacity={0} depthWrite={false} />
      </mesh>

      <RoundedBox args={[2, 0.08, 1.8]} radius={0.04} smoothness={4} position={[0, 0.04, 0]} receiveShadow castShadow>
        <meshStandardMaterial color={room.mat} roughness={0.94} />
      </RoundedBox>
      <mesh position={[0, 0.088, 0.82]}>
        <boxGeometry args={[1.9, 0.028, 0.1]} />
        <meshStandardMaterial color={accent ?? room.deskLeg} roughness={0.7} />
      </mesh>

      {/* error 红呼吸环（快、亮）；unprovisioned 琥珀呼吸环（更细更慢更暗，读作"待开通"非"告警"） */}
      {status === 'error' && <StatusPulseRing color={statusColor} />}
      {status === 'unprovisioned' && (
        <StatusPulseRing color={statusColor} ringArgs={UNPROVISIONED_PULSE_RING_ARGS} baseOpacity={0.34} speed={2.0} />
      )}
      {selected && <SelectionMarker color={room.ring} />}
      {(selected || hovered) && (
        <mesh position={[0, 0.014, 0]} rotation={[-Math.PI / 2, 0, 0]}>
          <ringGeometry args={[1.18, 1.28, 48]} />
          <meshBasicMaterial color={room.ring} transparent opacity={selected ? 0.86 : 0.32} />
        </mesh>
      )}

      <SeatWorkstation
        accent={accent}
        room={room}
        status={status}
        statusColor={statusColor}
        hasDeskPlant={hasDeskPlant}
        showPerson={Boolean(view.user && status === 'online')}
      />

      {(!compactLabel || selected || hovered || status === 'error') && (
        <SeatLabel
          label={label}
          status={status}
          statusColor={statusColor}
          statusLabel={statusLabel}
          selected={selected}
          hovered={hovered}
          device={view.device}
        />
      )}
    </group>
  )
})
