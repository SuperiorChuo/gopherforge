// 办公室空间建筑层：落地投影、地板底座、地砖纹理、通道动线、网格、墙体与角隅盆栽。
import { Grid, RoundedBox } from '@react-three/drei'
import { memo, useMemo } from 'react'
import type { ThreeEvent } from '@react-three/fiber'
import type { OverviewLayout } from '../officeLayout'
import { computeCirculationLanes, computeFloorTiles, createDropShadowTexture } from './geometry'
import { Plant } from './OfficeProps'
import type { RoomTheme } from './theme'
import { WallScreens } from './WallScreens'

const LEFT_CORNER_PLANT_POS: [number, number, number] = [0.45, 0.11, 0.48]

const FloorTiles = memo(function FloorTiles({ width, depth, color }: { width: number; depth: number; color: string }) {
  const tiles = useMemo(() => computeFloorTiles(width, depth), [depth, width])

  return tiles.map(([x, z]) => (
    <mesh key={`${x}-${z}`} position={[x, 0.006, z]} rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
      <planeGeometry args={[0.94, 0.94]} />
      <meshStandardMaterial color={color} roughness={0.96} />
    </mesh>
  ))
})

const CirculationLanes = memo(function CirculationLanes({
  overview,
  room,
}: {
  overview: OverviewLayout
  room: RoomTheme
}) {
  const lanes = useMemo(() => computeCirculationLanes(overview.pods), [overview.pods])

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
})

const RoomBase = memo(function RoomBase({
  width,
  depth,
  center,
  room,
}: {
  width: number
  depth: number
  center: [number, number, number]
  room: RoomTheme
}) {
  const shadowTexture = useMemo(() => createDropShadowTexture(room.shadow), [room.shadow])

  return (
    <>
      <mesh position={[center[0], -1.08, center[2]]} rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[(width + 3) * 1.9, (depth + 3) * 1.9]} />
        <meshBasicMaterial map={shadowTexture} transparent depthWrite={false} />
      </mesh>
      <RoundedBox
        args={[width + 2.4, 0.72, depth + 2.4]}
        radius={0.16}
        smoothness={6}
        position={[center[0], -0.37, center[2]]}
        castShadow
      >
        <meshStandardMaterial color={room.floorSide} roughness={0.9} />
      </RoundedBox>
    </>
  )
})

const RoomArchitecture = memo(function RoomArchitecture({
  width,
  depth,
  center,
  extent,
  editing,
  overview,
  room,
}: {
  width: number
  depth: number
  center: [number, number, number]
  extent: number
  editing: boolean
  overview: OverviewLayout
  room: RoomTheme
}) {
  const rightCornerPlantPos = useMemo<[number, number, number]>(() => [width - 0.7, 0.11, 0.5], [width])

  return (
    <>
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
      <Plant position={LEFT_CORNER_PLANT_POS} scale={1.35} room={room} />
      {width > 9 && <Plant position={rightCornerPlantPos} scale={1.15} room={room} />}
    </>
  )
})

export interface OfficeRoomProps {
  width: number
  depth: number
  center: [number, number, number]
  extent: number
  editing: boolean
  overview: OverviewLayout
  room: RoomTheme
  onFloorMove: (event: ThreeEvent<PointerEvent>) => void
  onEndDrag: () => void
  onAddSeatAt: (x: number, y: number) => void
}

export const OfficeRoom = memo(function OfficeRoom({
  width,
  depth,
  center,
  extent,
  editing,
  overview,
  room,
  onFloorMove,
  onEndDrag,
  onAddSeatAt,
}: OfficeRoomProps) {
  return (
    <>
      <RoomBase width={width} depth={depth} center={center} room={room} />
      <mesh
        rotation={[-Math.PI / 2, 0, 0]}
        position={[center[0], 0, center[2]]}
        receiveShadow
        onPointerMove={onFloorMove}
        onPointerUp={onEndDrag}
        onDoubleClick={(event) => {
          if (!editing) return
          event.stopPropagation()
          onAddSeatAt(event.point.x, event.point.z)
        }}
      >
        <planeGeometry args={[width + 2, depth + 2]} />
        <meshStandardMaterial color={room.floorTop} roughness={0.94} />
      </mesh>
      <RoomArchitecture
        width={width}
        depth={depth}
        center={center}
        extent={extent}
        editing={editing}
        overview={overview}
        room={room}
      />
    </>
  )
})
