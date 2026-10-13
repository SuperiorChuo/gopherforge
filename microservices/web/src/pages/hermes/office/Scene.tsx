// Hermes 3D 办公室渲染层。浏览态按真实部门组织展示岛，编辑态回到数据库物理坐标。
// 共享的布局算法与状态色在 officeLayout.ts。
import { Grid, Html, OrbitControls, OrthographicCamera, RoundedBox } from '@react-three/drei'
import { Bloom, EffectComposer } from '@react-three/postprocessing'
import { useTranslation } from 'react-i18next'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { ThreeEvent } from '@react-three/fiber'
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import * as THREE from 'three'
import type { HermesOfficeFloor, SeatStatus } from '@/api/ai/hermes-office'
import { seatStatus } from '@/api/ai/hermes-office'
import { formatDateTime } from '@/utils/format'
import { HEALTH_LABELS } from '../_shared'
import { buildOverviewLayout, SEAT_STATUS_COLORS } from './officeLayout'
import type { DepartmentPodLayout, OfficeDepartmentPod, OverviewLayout } from './officeLayout'
import type { SeatDraft } from './useSeatEditor'

// 保持 index.tsx 从 './Scene' 的既有 import 不变
export { SEAT_STATUS_COLORS } from './officeLayout'
export type { OfficeDepartmentPod } from './officeLayout'

// 场景在明暗站点主题下都保持浅色，确保它始终是一张可读的办公室运营图。
const ROOM_THEMES = {
  light: {
    bg: '#edf2f7',
    floorTop: '#f7f9fc',
    floorSide: '#cbd4df',
    tile: '#eef2f7',
    lane: '#f9fbfd',
    laneLine: '#d5dee8',
    grid: '#dbe3ec',
    gridSection: '#cbd6e2',
    wall: '#e1e7ee',
    wallSide: '#c7d0dc',
    baseboard: '#c3ceda',
    screenFrame: '#233044',
    screenGlow: '#6cc7e8',
    screenLine: '#a7e0ed',
    deskTop: '#ffffff',
    deskLeg: '#dce4ec',
    metal: '#aeb9c7',
    keyboard: '#4a5565',
    chair: '#596577',
    mat: '#fbfcfe',
    podTop: '#ffffff',
    podLabel: '#1f2937',
    ring: '#4f67d8',
    pot: '#d4dbe5',
    plant: '#42a879',
    skin: '#eac39e',
    shadow: 'rgba(71,85,105,0.34)',
  },
  dark: {
    bg: '#e8edf4',
    floorTop: '#f3f6fa',
    floorSide: '#c3ccd8',
    tile: '#e9eef4',
    lane: '#f6f8fb',
    laneLine: '#cfd9e4',
    grid: '#d4dde7',
    gridSection: '#c4cfdb',
    wall: '#dce3eb',
    wallSide: '#bec8d4',
    baseboard: '#bac5d2',
    screenFrame: '#1f2b3c',
    screenGlow: '#58b7da',
    screenLine: '#98d8e6',
    deskTop: '#fbfcfe',
    deskLeg: '#d7dfe8',
    metal: '#a4b0bf',
    keyboard: '#424d5e',
    chair: '#505d70',
    mat: '#f7f9fc',
    podTop: '#fbfcfe',
    podLabel: '#172033',
    ring: '#4f67d8',
    pot: '#cfd7e1',
    plant: '#3b9f70',
    skin: '#e4bb95',
    shadow: 'rgba(51,65,85,0.38)',
  },
} as const

type RoomTheme = { [K in keyof (typeof ROOM_THEMES)['light']]: string }

function useDropShadowTexture(color: string) {
  return useMemo(() => {
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
  }, [color])
}

function Plant({ position, scale = 1, room }: { position: [number, number, number]; scale?: number; room: RoomTheme }) {
  return (
    <group position={position} scale={scale}>
      {/* 桌面盆栽整体不投射阴影：叶球太小，阴影肉眼不可辨，省下每盆 4 个 caster */}
      <mesh>
        <cylinderGeometry args={[0.16, 0.12, 0.22, 16]} />
        <meshStandardMaterial color={room.pot} roughness={0.9} />
      </mesh>
      <mesh position={[0, 0.24, 0]}>
        <sphereGeometry args={[0.17, 16, 16]} />
        <meshStandardMaterial color={room.plant} roughness={0.8} />
      </mesh>
      <mesh position={[0.1, 0.34, 0.04]}>
        <sphereGeometry args={[0.11, 14, 14]} />
        <meshStandardMaterial color={room.plant} roughness={0.8} />
      </mesh>
      <mesh position={[-0.09, 0.33, -0.05]}>
        <sphereGeometry args={[0.09, 14, 14]} />
        <meshStandardMaterial color={room.plant} roughness={0.8} />
      </mesh>
    </group>
  )
}

function Person({ accent, room }: { accent: string; room: RoomTheme }) {
  return (
    <group position={[0, 0, 0.42]}>
      {/* 只保留躯干+头投射阴影（占位轮廓），头发与手臂细件不投射 */}
      <mesh position={[0, 0.58, 0]} castShadow>
        <capsuleGeometry args={[0.14, 0.24, 6, 14]} />
        <meshStandardMaterial color={accent} roughness={0.72} />
      </mesh>
      <mesh position={[0, 0.9, 0]} castShadow>
        <sphereGeometry args={[0.115, 18, 18]} />
        <meshStandardMaterial color={room.skin} roughness={0.76} />
      </mesh>
      <mesh position={[0, 0.965, -0.018]} scale={[1.02, 0.58, 1.02]}>
        <sphereGeometry args={[0.118, 18, 18]} />
        <meshStandardMaterial color="#354052" roughness={0.82} />
      </mesh>
      <mesh position={[-0.17, 0.62, -0.14]} rotation={[0.9, 0, 0.25]}>
        <capsuleGeometry args={[0.045, 0.22, 4, 10]} />
        <meshStandardMaterial color={accent} roughness={0.72} />
      </mesh>
      <mesh position={[0.17, 0.62, -0.14]} rotation={[0.9, 0, -0.25]}>
        <capsuleGeometry args={[0.045, 0.22, 4, 10]} />
        <meshStandardMaterial color={accent} roughness={0.72} />
      </mesh>
    </group>
  )
}

// StatusPulseRing 是 error/unprovisioned 工位脚下的呼吸环。加法混合 + toneMapped=false，
// 在浅色地板上读出"发光光晕"而非实心环；opacity 随脉冲呼吸。
function StatusPulseRing({
  color,
  ringArgs = [1.03, 1.18, 48] as [number, number, number],
  baseOpacity = 0.46,
  speed = 2.4,
}: {
  color: string
  ringArgs?: [number, number, number]
  baseOpacity?: number
  speed?: number
}) {
  const ref = useRef<THREE.Mesh>(null)
  useFrame(({ clock }) => {
    if (!ref.current) return
    const pulse = (Math.sin(clock.elapsedTime * speed) + 1) / 2
    ref.current.scale.setScalar(1 + pulse * 0.16)
    const material = ref.current.material as THREE.MeshBasicMaterial
    material.opacity = baseOpacity - pulse * (baseOpacity * 0.6)
  })
  return (
    <mesh ref={ref} position={[0, 0.02, 0]} rotation={[-Math.PI / 2, 0, 0]}>
      <ringGeometry args={ringArgs} />
      <meshBasicMaterial
        color={color}
        transparent
        opacity={baseOpacity}
        blending={THREE.AdditiveBlending}
        toneMapped={false}
        depthWrite={false}
      />
    </mesh>
  )
}

function SelectionMarker({ color }: { color: string }) {
  const ref = useRef<THREE.Group>(null)
  useFrame(({ clock }) => {
    if (!ref.current) return
    ref.current.position.y = 2.02 + Math.sin(clock.elapsedTime * 2.6) * 0.07
    ref.current.rotation.y = clock.elapsedTime * 0.55
  })
  return (
    <group ref={ref}>
      <mesh rotation={[Math.PI, 0, 0]}>
        <coneGeometry args={[0.13, 0.25, 4]} />
        {/* toneMapped=false + 提亮让选中锥越过 Bloom 阈值，全息感更强 */}
        <meshStandardMaterial color={color} emissive={color} emissiveIntensity={0.9} toneMapped={false} roughness={0.42} />
      </mesh>
      <mesh position={[0, 0.15, 0]} rotation={[-Math.PI / 2, 0, 0]}>
        <ringGeometry args={[0.15, 0.19, 32]} />
        <meshBasicMaterial color={color} transparent opacity={0.5} blending={THREE.AdditiveBlending} toneMapped={false} depthWrite={false} />
      </mesh>
    </group>
  )
}

interface SeatMeshProps {
  view: SeatDraft
  accent?: string
  selected: boolean
  editing: boolean
  room: RoomTheme
  compactLabel?: boolean
  displayPosition?: [number, number]
  displayRotation?: number
  onSelect: (seatId: number) => void
  onDragStart: (seatId: number) => void
}

function SeatMesh({
  view,
  accent,
  selected,
  editing,
  room,
  compactLabel = false,
  displayPosition,
  displayRotation,
  onSelect,
  onDragStart,
}: SeatMeshProps) {
  const { t } = useTranslation()
  const { seat } = view
  const [hovered, setHovered] = useState(false)
  const status = seatStatus(view)
  const statusColor = SEAT_STATUS_COLORS[status]
  const statusLabel = {
    online: '在线',
    offline: '离线',
    error: '异常',
    unprovisioned: '未开通',
    empty: '空位',
  }[status]
  const label = view.user ? view.user.nickname || view.user.username : seat.label || '空位'
  const hasDeskPlant = Math.abs(seat.id) % 3 === 0

  return (
    <group
      position={[displayPosition?.[0] ?? seat.x, 0, displayPosition?.[1] ?? seat.y]}
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
        <StatusPulseRing color={statusColor} ringArgs={[1.07, 1.16, 48]} baseOpacity={0.34} speed={2.0} />
      )}
      {selected && <SelectionMarker color={room.ring} />}
      {(selected || hovered) && (
        <mesh position={[0, 0.014, 0]} rotation={[-Math.PI / 2, 0, 0]}>
          <ringGeometry args={[1.18, 1.28, 48]} />
          <meshBasicMaterial color={room.ring} transparent opacity={selected ? 0.86 : 0.32} />
        </mesh>
      )}

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
      {hasDeskPlant && <Plant position={[0.55, 0.87, -0.5]} scale={0.5} room={room} />}
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

      {view.user && status === 'online' && <Person accent={accent ?? SEAT_STATUS_COLORS.online} room={room} />}

      {(!compactLabel || selected || hovered || status === 'error') && (
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
              {view.device && (selected || hovered) && (
                <span
                  className={`hermes-seat-pill-meta${HEALTH_LABELS[view.device.health_status].bad ? ' is-bad' : ''}`}
                >
                  {view.device.hermes_version ? `v${view.device.hermes_version}` : '—'}
                  {` · ${t(HEALTH_LABELS[view.device.health_status].label)}`}
                  {view.device.last_seen_at ? ` · ${formatDateTime(view.device.last_seen_at)}` : ''}
                </span>
              )}
            </span>
          </div>
        </Html>
      )}
    </group>
  )
}

function FloorTiles({ width, depth, color }: { width: number; depth: number; color: string }) {
  const tiles = useMemo(() => {
    const result: Array<[number, number]> = []
    if (width * depth > 4000) return result
    for (let x = 0; x < Math.floor(width); x += 1) {
      for (let z = 0; z < Math.floor(depth); z += 1) {
        if ((x * 7 + z * 11) % 9 === 0) result.push([x + 0.5, z + 0.5])
        if (result.length >= 300) return result
      }
    }
    return result
  }, [depth, width])

  return tiles.map(([x, z]) => (
    <mesh key={`${x}-${z}`} position={[x, 0.006, z]} rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
      <planeGeometry args={[0.94, 0.94]} />
      <meshStandardMaterial color={color} roughness={0.96} />
    </mesh>
  ))
}

function CirculationLanes({ overview, room }: { overview: OverviewLayout; room: RoomTheme }) {
  const lanes = useMemo(() => {
    const centers = (values: number[]) => {
      const unique = [...new Set(values.map((value) => Number(value.toFixed(3))))].sort((a, b) => a - b)
      return unique.slice(1).map((value, index) => (unique[index] + value) / 2)
    }
    return {
      vertical: centers(overview.pods.map((pod) => pod.x)),
      horizontal: centers(overview.pods.map((pod) => pod.z)),
    }
  }, [overview.pods])

  if (overview.pods.length < 2) return null

  const laneWidth = 1.34
  const lineOffset = laneWidth / 2 - 0.08
  return (
    <group>
      {lanes.vertical.map((x) => (
        <group key={`lane-x-${x}`} position={[x, 0.021, overview.depth / 2]}>
          <mesh rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
            <planeGeometry args={[laneWidth, overview.depth + 1.4]} />
            <meshStandardMaterial color={room.lane} roughness={0.96} />
          </mesh>
          {[-lineOffset, lineOffset].map((offset) => (
            <mesh key={offset} position={[offset, 0.006, 0]}>
              <boxGeometry args={[0.035, 0.012, overview.depth + 1.1]} />
              <meshBasicMaterial color={room.laneLine} transparent opacity={0.7} />
            </mesh>
          ))}
        </group>
      ))}
      {lanes.horizontal.map((z) => (
        <group key={`lane-z-${z}`} position={[overview.width / 2, 0.022, z]}>
          <mesh rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
            <planeGeometry args={[overview.width + 1.4, laneWidth]} />
            <meshStandardMaterial color={room.lane} roughness={0.96} />
          </mesh>
          {[-lineOffset, lineOffset].map((offset) => (
            <mesh key={offset} position={[0, 0.006, offset]}>
              <boxGeometry args={[overview.width + 1.1, 0.012, 0.035]} />
              <meshBasicMaterial color={room.laneLine} transparent opacity={0.7} />
            </mesh>
          ))}
        </group>
      ))}
    </group>
  )
}

function WallScreen({
  position,
  rotation = 0,
  room,
  variant,
}: {
  position: [number, number, number]
  rotation?: number
  room: RoomTheme
  variant: number
}) {
  const bars = [0.24, 0.42, 0.3, 0.56]
  return (
    <group position={position} rotation={[0, rotation, 0]}>
      <mesh castShadow>
        <boxGeometry args={[1.75, 0.9, 0.07]} />
        <meshStandardMaterial color={room.screenFrame} roughness={0.44} />
      </mesh>
      <mesh position={[0, 0, 0.039]}>
        <planeGeometry args={[1.55, 0.7]} />
        <meshStandardMaterial color={room.screenFrame} emissive={room.screenGlow} emissiveIntensity={0.16} />
      </mesh>
      <mesh position={[-0.38, 0.19, 0.045]}>
        <planeGeometry args={[0.58, 0.025]} />
        <meshBasicMaterial color={room.screenLine} transparent opacity={0.78} />
      </mesh>
      {bars.map((height, index) => (
        <mesh
          key={index}
          position={[-0.48 + index * 0.24, -0.22 + height / 2, 0.046]}
        >
          <planeGeometry args={[0.13, height]} />
          <meshBasicMaterial
            color={index === variant % bars.length ? SEAT_STATUS_COLORS.online : room.screenGlow}
            transparent
            opacity={index === variant % bars.length ? 0.88 : 0.54}
          />
        </mesh>
      ))}
      <mesh position={[0.45, 0.03, 0.046]} rotation={[0, 0, -0.35]}>
        <planeGeometry args={[0.48, 0.025]} />
        <meshBasicMaterial color={room.screenLine} transparent opacity={0.68} />
      </mesh>
      <mesh position={[0.49, -0.08, 0.046]} rotation={[0, 0, 0.22]}>
        <planeGeometry args={[0.4, 0.025]} />
        <meshBasicMaterial color={SEAT_STATUS_COLORS.unprovisioned} transparent opacity={0.62} />
      </mesh>
    </group>
  )
}

function WallScreens({ width, depth, room }: { width: number; depth: number; room: RoomTheme }) {
  const backCount = Math.min(3, Math.max(1, Math.floor(width / 9)))
  const leftCount = Math.min(2, Math.max(1, Math.floor(depth / 10)))
  return (
    <>
      {Array.from({ length: backCount }, (_, index) => {
        const x = ((index + 1) * width) / (backCount + 1)
        return (
          <WallScreen key={`back-${x}`} position={[x, 1.55, -1.025]} room={room} variant={index} />
        )
      })}
      {Array.from({ length: leftCount }, (_, index) => {
        const z = ((index + 1) * depth) / (leftCount + 1)
        return (
          <WallScreen
            key={`left-${z}`}
            position={[-1.025, 1.55, z]}
            rotation={Math.PI / 2}
            room={room}
            variant={index + 2}
          />
        )
      })}
    </>
  )
}


function PodStatusStrip({
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
  const segments = (['online', 'error', 'unprovisioned', 'offline', 'empty'] as SeatStatus[])
    .map((status) => ({ status, count: counts[status] }))
    .filter((segment) => segment.count > 0)
  const total = Math.max(1, segments.reduce((sum, segment) => sum + segment.count, 0))
  const stripWidth = Math.max(0.8, width - 0.72)
  const gap = 0.045
  const contentWidth = stripWidth - Math.max(0, segments.length - 1) * gap
  let cursor = -stripWidth / 2
  const layout = segments.map((segment) => {
    const segmentWidth = contentWidth * (segment.count / total)
    const x = cursor + segmentWidth / 2
    cursor += segmentWidth + gap
    return { ...segment, width: segmentWidth, x }
  })

  return (
    <group position={[0, 0.335, depth / 2 - 0.2]}>
      <mesh>
        <boxGeometry args={[stripWidth + 0.14, 0.026, 0.13]} />
        <meshStandardMaterial color={room.floorSide} roughness={0.82} />
      </mesh>
      {layout.map((segment) => (
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
}

function DepartmentPod({
  layout,
  seats,
  accents,
  selectedSeatId,
  compactLabels,
  room,
  onSelectSeat,
}: {
  layout: DepartmentPodLayout
  seats: Map<number, SeatDraft>
  accents: Map<number, string>
  selectedSeatId: number | null
  compactLabels: boolean
  room: RoomTheme
  onSelectSeat: (seatId: number | null) => void
}) {
  const { t } = useTranslation()
  const statusCounts = layout.seatIds.reduce<Record<SeatStatus, number>>(
    (counts, seatId) => {
      const view = seats.get(seatId)
      if (view) counts[seatStatus(view)] += 1
      return counts
    },
    { online: 0, offline: 0, error: 0, unprovisioned: 0, empty: 0 },
  )
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
              displayPosition={[position.x, position.z]}
              displayRotation={0}
              onSelect={onSelectSeat}
              onDragStart={() => undefined}
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
          <span className="hermes-department-label-count">{t('{{a}}/{{b}} 在线', { a: statusCounts.online, b: layout.seatIds.length })}</span>
          {riskCount > 0 && <span className="hermes-department-label-risk">{t('{{n}} 风险', { n: riskCount })}</span>}
        </div>
      </Html>
    </group>
  )
}

export interface SceneProps {
  floor: HermesOfficeFloor
  seats: SeatDraft[]
  selectedSeatId: number | null
  editing: boolean
  themeMode: 'light' | 'dark'
  accents: Map<number, string>
  pods: OfficeDepartmentPod[]
  onSelectSeat: (seatId: number | null) => void
  onMoveSeat: (seatId: number, x: number, y: number) => void
  onAddSeatAt: (x: number, y: number) => void
}

function OfficeCamera({
  center,
  width,
  depth,
  framingPadding = 1,
}: {
  center: [number, number, number]
  width: number
  depth: number
  framingPadding?: number
}) {
  const { size } = useThree()
  const cameraRef = useRef<THREE.OrthographicCamera>(null)
  const extent = Math.max(width, depth)
  const projectedWidth = (width + depth) * 0.72
  const projectedHeight = extent * 0.82
  const responsivePadding = size.width < 560 ? framingPadding * 1.18 : framingPadding
  const minimumZoom = size.width < 560 ? 6.25 : 9
  const zoom = Math.max(
    minimumZoom,
    Math.min(
      80,
      Math.min(size.width / (projectedWidth + 4.5), size.height / (projectedHeight + 4.5)) / responsivePadding,
    ),
  )
  const position = useMemo<[number, number, number]>(
    () => [center[0] + extent * 1.15, extent * 1.05, center[2] + extent * 1.25],
    [center, extent],
  )

  useEffect(() => {
    const camera = cameraRef.current
    if (!camera) return
    camera.lookAt(center[0], center[1], center[2])
    camera.updateProjectionMatrix()
  }, [center])

  return (
    <OrthographicCamera
      ref={cameraRef}
      makeDefault
      position={position}
      zoom={zoom}
      left={size.width / -2}
      right={size.width / 2}
      top={size.height / 2}
      bottom={size.height / -2}
      near={0.1}
      far={Math.max(240, extent * 8)}
    />
  )
}

export default function Scene({
  floor,
  seats,
  selectedSeatId,
  editing,
  themeMode,
  accents,
  pods,
  onSelectSeat,
  onMoveSeat,
  onAddSeatAt,
}: SceneProps) {
  const [draggingId, setDraggingId] = useState<number | null>(null)
  const room = ROOM_THEMES[themeMode]
  const overview = useMemo(() => buildOverviewLayout(pods), [pods])
  const seatById = useMemo(() => new Map(seats.map((view) => [view.seat.id, view])), [seats])
  const width = editing ? floor.width : overview.width
  const depth = editing ? floor.depth : overview.depth
  const extent = Math.max(width, depth)
  const shadowTexture = useDropShadowTexture(room.shadow)
  const compactLabels = !editing && overview.pods.length > 3 && seats.length > 24
  const framingPadding = editing ? 1 : overview.pods.length === 1 ? 0.68 : 1.02
  const center = useMemo<[number, number, number]>(() => [width / 2, 0, depth / 2], [depth, width])
  const cameraTarget = useMemo<[number, number, number]>(() => {
    if (editing || overview.pods.length === 0) return center
    return [
      overview.pods.reduce((sum, pod) => sum + pod.x, 0) / overview.pods.length,
      0.5,
      overview.pods.reduce((sum, pod) => sum + pod.z, 0) / overview.pods.length,
    ]
  }, [center, editing, overview.pods])

  const handleFloorMove = (event: ThreeEvent<PointerEvent>) => {
    if (draggingId == null) return
    event.stopPropagation()
    onMoveSeat(draggingId, event.point.x, event.point.z)
  }
  const endDrag = () => setDraggingId(null)
  const shadowRange = extent * 0.9

  return (
    <Canvas
      shadows
      dpr={[1, 2]}
      gl={{ antialias: true, powerPreference: 'high-performance' }}
      onPointerMissed={() => onSelectSeat(null)}
      onPointerUp={endDrag}
      onPointerLeave={endDrag}
      style={{ width: '100%', height: '100%' }}
    >
      <color attach="background" args={[room.bg]} />
      <OfficeCamera center={cameraTarget} width={width} depth={depth} framingPadding={framingPadding} />

      <hemisphereLight intensity={1.15} color="#ffffff" groundColor={room.floorSide} />
      {/* ambient 略提以承接冷色补光，主光略降防地板过曝（补光无阴影，零成本） */}
      <ambientLight intensity={0.6} />
      <directionalLight
        position={[width * 0.78, extent * 1.2, depth * 0.18]}
        intensity={1.55}
        castShadow
        shadow-mapSize={[2048, 2048]}
        shadow-camera-left={-shadowRange}
        shadow-camera-right={shadowRange}
        shadow-camera-top={shadowRange}
        shadow-camera-bottom={-shadowRange}
        shadow-camera-far={extent * 4}
        shadow-bias={-0.0003}
      />
      {/* 冷色补光：主光对侧，勾出 pod 边缘与后墙层次 */}
      <directionalLight position={[width * 0.2, extent * 0.7, depth * 0.85]} intensity={0.5} color="#dbe7ff" />
      {/* 极淡雾：远景边缘柔和淡出（色同背景，防压扁网格；若观感发灰则删掉这两行） */}
      <fog attach="fog" args={[room.bg, extent * 1.6, extent * 3.5]} />

      <mesh position={[center[0], -1.08, center[2]]} rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[(width + 3) * 1.9, (depth + 3) * 1.9]} />
        <meshBasicMaterial map={shadowTexture} transparent depthWrite={false} />
      </mesh>
      <RoundedBox args={[width + 2.4, 0.72, depth + 2.4]} radius={0.16} smoothness={6} position={[center[0], -0.37, center[2]]} castShadow>
        <meshStandardMaterial color={room.floorSide} roughness={0.9} />
      </RoundedBox>
      <mesh
        rotation={[-Math.PI / 2, 0, 0]}
        position={[center[0], 0, center[2]]}
        receiveShadow
        onPointerMove={handleFloorMove}
        onPointerUp={endDrag}
        onDoubleClick={(event) => {
          if (!editing) return
          event.stopPropagation()
          onAddSeatAt(event.point.x, event.point.z)
        }}
      >
        <planeGeometry args={[width + 2, depth + 2]} />
        <meshStandardMaterial color={room.floorTop} roughness={0.94} />
      </mesh>

      <FloorTiles width={width} depth={depth} color={room.tile} />
      {!editing && <CirculationLanes overview={overview} room={room} />}
      <Grid
        position={[center[0], 0.013, center[2]]}
        args={[width + 2, depth + 2]}
        cellSize={1}
        cellThickness={editing ? 0.6 : 0.36}
        cellColor={room.grid}
        sectionSize={5}
        sectionThickness={editing ? 1.1 : 0.65}
        sectionColor={room.gridSection}
        fadeDistance={extent * 4}
        infiniteGrid={false}
      />

      <mesh position={[center[0], 1.32, -1.25]} castShadow receiveShadow>
        <boxGeometry args={[width + 3, 2.64, 0.5]} />
        <meshStandardMaterial color={room.wall} roughness={0.92} />
      </mesh>
      <mesh position={[-1.25, 1.32, center[2]]} castShadow receiveShadow>
        <boxGeometry args={[0.5, 2.64, depth + 3]} />
        <meshStandardMaterial color={room.wallSide} roughness={0.92} />
      </mesh>
      <mesh position={[center[0], 0.1, -0.985]}>
        <boxGeometry args={[width + 2, 0.2, 0.04]} />
        <meshStandardMaterial color={room.baseboard} roughness={0.86} />
      </mesh>
      <mesh position={[-0.985, 0.1, center[2]]}>
        <boxGeometry args={[0.04, 0.2, depth + 2]} />
        <meshStandardMaterial color={room.baseboard} roughness={0.86} />
      </mesh>
      <mesh position={[width + 1.12, 0.32, center[2]]} castShadow>
        <boxGeometry args={[0.28, 0.64, Math.max(2.2, depth * 0.45)]} />
        <meshStandardMaterial color={room.wall} roughness={0.92} />
      </mesh>
      <mesh position={[width * 0.24, 0.32, depth + 1.12]} castShadow>
        <boxGeometry args={[Math.max(2.2, width * 0.38), 0.64, 0.28]} />
        <meshStandardMaterial color={room.wallSide} roughness={0.92} />
      </mesh>

      <WallScreens width={width} depth={depth} room={room} />
      <Plant position={[0.45, 0.11, 0.48]} scale={1.35} room={room} />
      {width > 9 && <Plant position={[width - 0.7, 0.11, 0.5]} scale={1.15} room={room} />}

      {editing
        ? seats.map((view) => (
            <SeatMesh
              key={view.seat.id}
              view={view}
              accent={accents.get(view.seat.id)}
              selected={view.seat.id === selectedSeatId}
              editing
              room={room}
              onSelect={onSelectSeat}
              onDragStart={setDraggingId}
            />
          ))
        : overview.pods.map((pod) => (
            <DepartmentPod
              key={pod.key}
              layout={pod}
              seats={seatById}
              accents={accents}
              selectedSeatId={selectedSeatId}
              compactLabels={compactLabels}
              room={room}
              onSelectSeat={onSelectSeat}
            />
          ))}

      <OrbitControls
        makeDefault
        enabled={draggingId == null}
        enableDamping
        dampingFactor={0.08}
        target={cameraTarget}
        minPolarAngle={Math.PI / 5}
        maxPolarAngle={Math.PI / 2.4}
        minZoom={Math.max(4, 12 / Math.max(extent, 1))}
        maxZoom={180}
      />

      {/* Bloom 后处理：亮屏/脉冲环/信标/选中锥的"科技光晕"。保守参数 + mipmapBlur
          防止浅色地板雾化。若弱 GPU 拖帧：先降 multisampling=0，仍拖则整体注释掉
          这一块（回退到纯加法辉光，见 StatusPulseRing/SelectionMarker）。 */}
      <EffectComposer multisampling={4}>
        <Bloom intensity={0.45} luminanceThreshold={0.9} luminanceSmoothing={0.2} mipmapBlur radius={0.8} />
      </EffectComposer>
    </Canvas>
  )
}
