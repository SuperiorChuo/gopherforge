import { useCallback, useMemo, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'

/**
 * 列表页搜索参数与 URL query 双向同步：
 * 刷新/返回不丢条件，链接可直接分享当前筛选视图。
 * numericKeys 里的键从 URL 读回时转为 number（其余保持字符串）。
 */
export function useUrlParams<T extends object>(
  defaults: T,
  numericKeys: string[] = ['page', 'page_size', 'status', 'type'],
): [T, (next: T) => void] {
  const [searchParams, setSearchParams] = useSearchParams()
  const defaultsRef = useRef(defaults)
  const numericRef = useRef(numericKeys)

  const params = useMemo(() => {
    const result: Record<string, unknown> = { ...(defaultsRef.current as Record<string, unknown>) }
    searchParams.forEach((value, key) => {
      if (value === '') return
      if (numericRef.current.includes(key)) {
        const n = Number(value)
        if (!Number.isNaN(n)) result[key] = n
      } else {
        result[key] = value
      }
    })
    return result as T
  }, [searchParams])

  const setParams = useCallback(
    (next: T) => {
      const sp = new URLSearchParams()
      Object.entries(next).forEach(([k, v]) => {
        if (v === undefined || v === null || v === '') return
        sp.set(k, String(v))
      })
      setSearchParams(sp, { replace: true })
    },
    [setSearchParams],
  )

  return [params, setParams]
}
