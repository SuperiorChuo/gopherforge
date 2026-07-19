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
}

/**
 * 轮询后台任务状态：GEO 跑题、SEO 排名检查共用。
 * - in-flight 标志防请求堆叠（上一次未返回则跳过本 tick）
 * - 组件卸载 / taskId 变化时自动清理
 * - 连续 3 次请求失败自动停止
 */
export function usePollingTask<T>({ taskId, fetcher, isDone, intervalMs = 2000, onTick, onFinish }: Options<T>) {
  const [data, setData] = useState<T | null>(null)
  const [polling, setPolling] = useState(false)
  const inFlight = useRef(false)
  const failures = useRef(0)
  const cbRef = useRef({ fetcher, isDone, onTick, onFinish })
  cbRef.current = { fetcher, isDone, onTick, onFinish }

  useEffect(() => {
    if (taskId == null) {
      setPolling(false)
      return
    }
    setData(null)
    setPolling(true)
    failures.current = 0
    let stopped = false
    let timer: ReturnType<typeof setInterval> | null = null

    const tick = async () => {
      if (inFlight.current || stopped) return
      inFlight.current = true
      try {
        const t = await cbRef.current.fetcher(taskId)
        failures.current = 0
        if (stopped) return
        setData(t)
        cbRef.current.onTick?.(t)
        if (cbRef.current.isDone(t)) {
          stopped = true
          if (timer) clearInterval(timer)
          setPolling(false)
          cbRef.current.onFinish?.(t)
        }
      } catch {
        failures.current += 1
        if (failures.current >= 3) {
          stopped = true
          if (timer) clearInterval(timer)
          setPolling(false)
        }
      } finally {
        inFlight.current = false
      }
    }

    void tick()
    timer = setInterval(() => void tick(), intervalMs)
    return () => {
      stopped = true
      if (timer) clearInterval(timer)
    }
  }, [taskId, intervalMs])

  return { data, polling }
}
