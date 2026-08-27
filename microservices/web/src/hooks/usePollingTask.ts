import { useEffect, useRef, useState } from 'react'

type Options<T> = {
  /** null = 不轮询 */
  taskId: number | null
  fetcher: (id: number) => Promise<T>
  /** done/failed 都应返回 true */
  isDone: (t: T) => boolean
  intervalMs?: number
  onTick?: (t: T) => void
  onFinish?: (t: T) => void
  /** 连续请求失败自动停止时回调；调用方应清掉 taskId，否则同 id 无法重启轮询 */
  onError?: () => void
}

/**
 * 轮询后台任务状态：GEO 跑题、SEO 排名检查共用。
 * - in-flight 标志防请求堆叠（上一次未返回则跳过本 tick）
 * - 组件卸载 / taskId 变化时自动清理
 * - 连续 3 次请求失败自动停止并回调 onError
 */
export function usePollingTask<T>({ taskId, fetcher, isDone, intervalMs = 2000, onTick, onFinish, onError }: Options<T>) {
  const [data, setData] = useState<T | null>(null)
  const [polling, setPolling] = useState(false)
  const cbRef = useRef({ fetcher, isDone, onTick, onFinish, onError })
  // latest-ref 写入放 effect（声明在主 effect 前，先于首轮 tick 执行）；
  // render 期写 ref 在 StrictMode 弃用渲染下会残留错值。
  useEffect(() => {
    cbRef.current = { fetcher, isDone, onTick, onFinish, onError }
  })

  useEffect(() => {
    if (taskId == null) {
      setPolling(false)
      return
    }
    setData(null)
    setPolling(true)
    // in-flight/failures 必须是本 effect 的局部量：跨 taskId 用 ref 会让
    // 新任务的首轮 tick 被上一个任务未返回的请求阻塞
    let inFlight = false
    let failures = 0
    let stopped = false
    let timer: ReturnType<typeof setInterval> | null = null

    const tick = async () => {
      if (inFlight || stopped) return
      inFlight = true
      try {
        const t = await cbRef.current.fetcher(taskId)
        failures = 0
        if (stopped) return
        setData(t)
        cbRef.current.onTick?.(t)
        if (cbRef.current.isDone(t)) {
          stopped = true
          stop()
          setPolling(false)
          cbRef.current.onFinish?.(t)
        }
      } catch {
        failures += 1
        if (failures >= 3 && !stopped) {
          stopped = true
          stop()
          setPolling(false)
          cbRef.current.onError?.()
        }
      } finally {
        inFlight = false
      }
    }

    // 后台标签页停表：任务有终态会自停，但用户切走后仍以 2s 打接口纯属浪费。
    // 回前台立刻补一次再恢复节奏（终态可能在后台期间已经到达）。
    const start = () => {
      if (timer === null && !stopped) {
        timer = setInterval(() => void tick(), intervalMs)
      }
    }
    const stop = () => {
      if (timer !== null) {
        clearInterval(timer)
        timer = null
      }
    }
    const onVisibility = () => {
      if (stopped) return
      if (document.visibilityState === 'visible') {
        void tick()
        start()
      } else {
        stop()
      }
    }

    void tick()
    if (document.visibilityState === 'visible') {
      start()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stopped = true
      stop()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [taskId, intervalMs])

  return { data, polling }
}
