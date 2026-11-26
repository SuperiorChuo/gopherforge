// 办公室后墙与侧墙的运营监控大屏装饰组件。
import { memo, useMemo } from 'react'
import { SEAT_STATUS_COLORS } from '../officeLayout'
import type { RoomTheme } from './theme'

const WALL_SCREEN_BARS = [0.24, 0.42, 0.3, 0.56] as const

export interface WallScreenProps {
  position: [number, number, number]
  rotation?: number
  room: RoomTheme
  variant: number
}

export const WallScreen = memo(function WallScreen({ position, rotation = 0, room, variant }: WallScreenProps) {
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
      {WALL_SCREEN_BARS.map((height, index) => (
        <mesh key={index} position={[-0.48 + index * 0.24, -0.22 + height / 2, 0.046]}>
          <planeGeometry args={[0.13, height]} />
          <meshBasicMaterial
            color={index === variant % WALL_SCREEN_BARS.length ? SEAT_STATUS_COLORS.online : room.screenGlow}
            transparent
            opacity={index === variant % WALL_SCREEN_BARS.length ? 0.88 : 0.54}
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
})

export interface WallScreensProps {
  width: number
  depth: number
  room: RoomTheme
}

export const WallScreens = memo(function WallScreens({ width, depth, room }: WallScreensProps) {
  const { backPositions, leftPositions } = useMemo(() => {
    const backCount = Math.min(3, Math.max(1, Math.floor(width / 9)))
    const leftCount = Math.min(2, Math.max(1, Math.floor(depth / 10)))
    return {
      backPositions: Array.from({ length: backCount }, (_, index): [number, number, number] => [
        ((index + 1) * width) / (backCount + 1),
        1.55,
        -1.025,
      ]),
      leftPositions: Array.from({ length: leftCount }, (_, index): [number, number, number] => [
        -1.025,
        1.55,
        ((index + 1) * depth) / (leftCount + 1),
      ]),
    }
  }, [depth, width])

  return (
    <>
      {backPositions.map((position, index) => (
        <WallScreen key={`back-${position[0]}`} position={position} room={room} variant={index} />
      ))}
      {leftPositions.map((position, index) => (
        <WallScreen
          key={`left-${position[2]}`}
          position={position}
          rotation={Math.PI / 2}
          room={room}
          variant={index + 2}
        />
      ))}
    </>
  )
})
