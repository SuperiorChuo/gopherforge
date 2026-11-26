// Hermes 3D 办公室渲染层入口。浏览态按真实部门组织展示岛，编辑态回到数据库物理坐标。
// 共享常量与类型在 constants.ts，布局算法在 officeLayout.ts，3D 子模块拆分于 ./scene/ 目录。
import { OrbitControls } from '@react-three/drei'
import { Bloom, EffectComposer } from '@react-three/postprocessing'
import { useCallback, useMemo, useState } from 'react'
import type { ThreeEvent } from '@react-three/fiber'
import { Canvas } from '@react-three/fiber'
import type { HermesOfficeFloor } from '@/api/ai/hermes-office'
import type { OfficeDepartmentPod } from './constants'
import { buildOverviewLayout } from './officeLayout'
import type { SeatDraft } from './useSeatEditor'
import { DepartmentPod } from './scene/DepartmentPod'
import { computeCameraTarget } from './scene/geometry'
import { OfficeCamera } from './scene/OfficeCamera'
import { OfficeRoom } from './scene/OfficeRoom'
import { SeatMesh } from './scene/SeatMesh'
import { ROOM_THEMES } from './scene/theme'

// 保留从 './Scene' 的 re-export 以兼容外部引入；非 3D 调用方应直接从 './constants' 引入。
export { SEAT_STATUS_COLORS } from './constants'
export type { OfficeDepartmentPod } from './constants'

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
  const compactLabels = !editing && overview.pods.length > 3 && seats.length > 24
  const framingPadding = editing ? 1 : overview.pods.length === 1 ? 0.68 : 1.02
  const center = useMemo<[number, number, number]>(() => [width / 2, 0, depth / 2], [depth, width])
  const cameraTarget = useMemo(
    () => computeCameraTarget(center, editing, overview.pods),
    [center, editing, overview.pods],
  )

  const handleFloorMove = useCallback(
    (event: ThreeEvent<PointerEvent>) => {
      if (draggingId == null) return
      event.stopPropagation()
      onMoveSeat(draggingId, event.point.x, event.point.z)
    },
    [draggingId, onMoveSeat],
  )
  const endDrag = useCallback(() => setDraggingId(null), [])
  const handlePointerMissed = useCallback(() => onSelectSeat(null), [onSelectSeat])
  const shadowRange = extent * 0.9

  return (
    <Canvas
      shadows
      dpr={[1, 2]}
      gl={{ antialias: true, powerPreference: 'high-performance' }}
      onPointerMissed={handlePointerMissed}
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

      <OfficeRoom
        width={width}
        depth={depth}
        center={center}
        extent={extent}
        editing={editing}
        overview={overview}
        room={room}
        onFloorMove={handleFloorMove}
        onEndDrag={endDrag}
        onAddSeatAt={onAddSeatAt}
      />

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
              selectedSeatId={selectedSeatId != null && pod.seatIds.includes(selectedSeatId) ? selectedSeatId : null}
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
