import { useCallback, useEffect, useRef, useState } from 'react'
import {
  applyTableQueryError,
  applyTableQueryResult,
  beginTableQueryReload,
  createInitialTableQueryState,
  nextTableQueryRequestID,
  patchTableQueryList,
  type TableQueryState,
} from './table-query'

export type { TableQueryState }

export type UseTableQueryOptions<T, P> = {
  params: P
  fetcher: (params: P) => Promise<{ list: T[]; total: number }>
  onError?: () => void
}

/** 统一列表加载、刷新、旧数据保留、过期响应丢弃和错误态；筛选/分页状态仍由页面持有。 */
export function useTableQuery<T, P>({ params, fetcher, onError }: UseTableQueryOptions<T, P>) {
  const [state, setState] = useState<TableQueryState<T>>(createInitialTableQueryState)
  const requestIDRef = useRef(0)

  const reload = useCallback(async (options?: { silent?: boolean }) => {
    const requestID = nextTableQueryRequestID(requestIDRef.current)
    requestIDRef.current = requestID
    if (!options?.silent) {
      setState((current) => beginTableQueryReload(current, options))
    }
    try {
      const result = await fetcher(params)
      setState((current) => applyTableQueryResult(current, requestID, requestIDRef.current, result))
    } catch {
      const latestID = requestIDRef.current
      setState((current) => applyTableQueryError(current, requestID, latestID, options))
      if (requestID === latestID && !options?.silent) onError?.()
    }
  }, [fetcher, onError, params])

  const patchList = useCallback((updater: (list: T[]) => T[]) => {
    setState((current) => patchTableQueryList(current, updater))
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  return { ...state, reload, patchList }
}
