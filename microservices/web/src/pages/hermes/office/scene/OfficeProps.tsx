// 办公室 3D 装饰与人物模型（桌面/角落盆栽、在岗员工人物）。
import { memo } from 'react'
import type { RoomTheme } from './theme'

export interface PlantProps {
  position: [number, number, number]
  scale?: number
  room: RoomTheme
}

export const Plant = memo(function Plant({ position, scale = 1, room }: PlantProps) {
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
})

export interface PersonProps {
  accent: string
  room: RoomTheme
}

export const Person = memo(function Person({ accent, room }: PersonProps) {
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
})
