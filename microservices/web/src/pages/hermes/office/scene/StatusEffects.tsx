// 工位动态发光提示层：异常/未开通脉冲呼吸环与选中悬浮指示锥。
import { memo, useRef } from 'react'
import { useFrame } from '@react-three/fiber'
import * as THREE from 'three'
import { DEFAULT_PULSE_RING_ARGS } from './geometry'

export interface StatusPulseRingProps {
  color: string
  ringArgs?: [number, number, number]
  baseOpacity?: number
  speed?: number
}

// StatusPulseRing 是 error/unprovisioned 工位脚下的呼吸环。加法混合 + toneMapped=false，
// 在浅色地板上读出"发光光晕"而非实心环；opacity 随脉冲呼吸。
export const StatusPulseRing = memo(function StatusPulseRing({
  color,
  ringArgs = DEFAULT_PULSE_RING_ARGS,
  baseOpacity = 0.46,
  speed = 2.4,
}: StatusPulseRingProps) {
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
})

export interface SelectionMarkerProps {
  color: string
}

export const SelectionMarker = memo(function SelectionMarker({ color }: SelectionMarkerProps) {
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
        <meshBasicMaterial
          color={color}
          transparent
          opacity={0.5}
          blending={THREE.AdditiveBlending}
          toneMapped={false}
          depthWrite={false}
        />
      </mesh>
    </group>
  )
})
