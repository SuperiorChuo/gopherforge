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

export type TreeQueryState<T> = {
  tree: T[]
  loading: boolean
  error: boolean
}

export type UseTreeQueryOptions<T, P = void> = {
  params?: P
  fetcher: (params: P) => Promise<T[]>
  onError?: () => void
  enabled?: boolean
}

/** 统一树加载、刷新、旧数据保留和过期响应丢弃；展开键仍由页面持有。 */
export function useTreeQuery<T, P = void>({
  params,
  fetcher,
  onError,
  enabled = true,
}: UseTreeQueryOptions<T, P>) {
  const [state, setState] = useState<TableQueryState<T>>(createInitialTableQueryState)
  const requestIDRef = useRef(0)

  const reload = useCallback(async (options?: { silent?: boolean }) => {
    if (!enabled) return
    const requestID = nextTableQueryRequestID(requestIDRef.current)
    requestIDRef.current = requestID
    if (!options?.silent) {
      setState((current) => beginTableQueryReload(current, options))
    }
    try {
      const tree = await fetcher(params as P)
      setState((current) => applyTableQueryResult(current, requestID, requestIDRef.current, {
        list: tree,
        total: tree.length,
      }))
    } catch {
      const latestID = requestIDRef.current
      setState((current) => applyTableQueryError(current, requestID, latestID, options))
      if (requestID === latestID && !options?.silent) onError?.()
    }
  }, [enabled, fetcher, onError, params])

  const patchTree = useCallback((updater: (tree: T[]) => T[]) => {
    setState((current) => patchTableQueryList(current, updater))
  }, [])

  useEffect(() => {
    if (!enabled) return
    let active = true
    queueMicrotask(() => {
      if (active) void reload()
    })
    return () => {
      active = false
    }
  }, [enabled, reload])

  return {
    tree: state.list,
    loading: state.loading,
    error: state.error,
    reload,
    patchTree,
  } satisfies TreeQueryState<T> & { reload: typeof reload; patchTree: typeof patchTree }
}
