// 自适应正交等轴测相机，根据楼层/总览包络自动计算缩放与取景中心。
import { OrthographicCamera } from '@react-three/drei'
import { memo, useEffect, useMemo, useRef } from 'react'
import { useThree } from '@react-three/fiber'
import type * as THREE from 'three'
import { computeCameraFraming } from './geometry'

export interface OfficeCameraProps {
  center: [number, number, number]
  width: number
  depth: number
  framingPadding?: number
}

export const OfficeCamera = memo(function OfficeCamera({
  center,
  width,
  depth,
  framingPadding = 1,
}: OfficeCameraProps) {
  const { size } = useThree()
  const cameraRef = useRef<THREE.OrthographicCamera>(null)
  const { extent, zoom, far } = computeCameraFraming(
    width,
    depth,
    size.width,
    size.height,
    framingPadding,
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
      far={far}
    />
  )
})
