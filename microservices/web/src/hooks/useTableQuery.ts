import { useCallback, useEffect, useState } from 'react'

export type TableQueryState<T> = {
  list: T[]
  total: number
  loading: boolean
  error: boolean
}

export type UseTableQueryOptions<T, P> = {
  params: P
  fetcher: (params: P) => Promise<{ list: T[]; total: number }>
  onError?: () => void
}

/** 统一列表加载、刷新、旧数据保留和错误态；筛选/分页状态仍由页面持有。 */
export function useTableQuery<T, P>({ params, fetcher, onError }: UseTableQueryOptions<T, P>) {
  const [state, setState] = useState<TableQueryState<T>>({
    list: [],
    total: 0,
    loading: false,
    error: false,
  })

  const reload = useCallback(async (options?: { silent?: boolean }) => {
    if (!options?.silent) {
      setState((current) => ({ ...current, loading: true, error: false }))
    }
    try {
      const result = await fetcher(params)
      setState({ list: result.list, total: result.total, loading: false, error: false })
    } catch {
      setState((current) => ({ ...current, loading: false, error: options?.silent ? current.error : true }))
      if (!options?.silent) onError?.()
    }
  }, [fetcher, onError, params])

  useEffect(() => {
    void reload()
  }, [reload])

  return { ...state, reload }
}
