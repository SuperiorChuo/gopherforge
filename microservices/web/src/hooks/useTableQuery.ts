import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import {
  applyTableQueryError,
  applyTableQueryResult,
  beginTableQueryReload,
  createInitialTableQueryState,
  nextTableQueryRequestID,
  patchTableQueryList,
  stabilizeTableQueryParams,
  type TableQueryState,
} from './table-query'

export type { TableQueryState }

export type UseTableQueryOptions<T, P> = {
  params: P
  fetcher: (params: P) => Promise<{ list: T[]; total: number }>
  onError?: () => void
  enabled?: boolean
}

/** 统一列表加载、刷新、旧数据保留、过期响应丢弃和错误态；筛选/分页状态仍由页面持有。 */
export function useTableQuery<T, P>({ params, fetcher, onError, enabled = true }: UseTableQueryOptions<T, P>) {
  const [state, setState] = useState<TableQueryState<T>>(createInitialTableQueryState)
  const [stableParams, setStableParams] = useState(params)
  const requestIDRef = useRef(0)
  const onErrorRef = useRef(onError)
  const nextStableParams = stabilizeTableQueryParams(stableParams, params)
  if (!Object.is(nextStableParams, stableParams)) {
    setStableParams(nextStableParams)
  }
  useLayoutEffect(() => {
    onErrorRef.current = onError
  }, [onError])

  const reload = useCallback(async (options?: { silent?: boolean }) => {
    if (!enabled) return
    const currentRequestID = nextTableQueryRequestID(requestIDRef.current)
    requestIDRef.current = currentRequestID
    if (!options?.silent) {
      setState((current) => beginTableQueryReload(current, options))
    }
    try {
      const result = await fetcher(stableParams)
      setState((current) => applyTableQueryResult(current, currentRequestID, requestIDRef.current, result))
    } catch {
      const latestID = requestIDRef.current
      setState((current) => applyTableQueryError(current, currentRequestID, latestID, options))
      if (currentRequestID === latestID && !options?.silent) onErrorRef.current?.()
    }
  }, [enabled, fetcher, stableParams])

  const patchList = useCallback((updater: (list: T[]) => T[]) => {
    setState((current) => patchTableQueryList(current, updater))
  }, [])

  useEffect(() => {
    if (!enabled) return
    void reload()
  }, [enabled, reload])

  return { ...state, reload, patchList }
}
