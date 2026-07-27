import { useEffect, useRef } from 'react'

// 带页面可见性感知的轮询：后台标签页暂停计时器，回到前台立即补一次再恢复
// 节奏。监控/坐席页 5-10s 的轮询挂后台一小时就是数百次白打的请求，统一止血。
// callback 走 ref，调用方无需 useCallback 也不会因闭包重建重置计时器。
// immediate=false 用于「挂载时另有非静默首拉、轮询只做静默刷新」的页面。
export function useVisibilityInterval(callback: () => void, delayMs: number, immediate = true) {
  const cbRef = useRef(callback)
  cbRef.current = callback

  useEffect(() => {
    let timer: ReturnType<typeof setInterval> | null = null
    const start = () => {
      if (timer === null) {
        timer = setInterval(() => cbRef.current(), delayMs)
      }
    }
    const stop = () => {
      if (timer !== null) {
        clearInterval(timer)
        timer = null
      }
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        cbRef.current()
        start()
      } else {
        stop()
      }
    }

    if (immediate) {
      cbRef.current()
    }
    if (document.visibilityState === 'visible') {
      start()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stop()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [delayMs, immediate])
}
