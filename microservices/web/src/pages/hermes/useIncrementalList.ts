import { useCallback, useEffect, useRef, useState } from 'react'
import type { PageResponse } from '@/types'

export type HermesIncrementalFetcher<T> = (
  page: number,
  pageSize: number,
  keyword?: string,
) => Promise<PageResponse<T>>

/**
 * Hermes 域内辅助选择器数据源：按页追加加载，滚动触底拉下一页；可选用 keyword 远程搜索。
 * 只服务 Hermes 域内列表（策略/集群/制品/节点/服务器），不承载系统用户等域外契约。
 * 竞态防护：reload 取世代号，过期响应丢弃；loadMore 用 ref 同步锁，连点滚动不会双请求跳页。
 */
export function useIncrementalList<T>(fetcher: HermesIncrementalFetcher<T>, pageSize = 100) {
  const [items, setItems] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const loadingRef = useRef(false)
  const generationRef = useRef(0)
  const keywordRef = useRef('')
  const pageRef = useRef(0)

  const reload = useCallback(async (nextKeyword?: string) => {
    const generation = generationRef.current + 1
    generationRef.current = generation
    keywordRef.current = nextKeyword ?? ''
    loadingRef.current = true
    setLoading(true)
    try {
      const result = await fetcher(1, pageSize, keywordRef.current)
      if (generation !== generationRef.current) return
      setItems(result.list ?? [])
      setTotal(result.total ?? 0)
      pageRef.current = 1
    } catch {
      // 请求层已提示服务端消息；保留旧列表，避免选择器内容闪空
    } finally {
      if (generation === generationRef.current) {
        loadingRef.current = false
        setLoading(false)
      }
    }
  }, [fetcher, pageSize])

  const loadMore = useCallback(async () => {
    if (loadingRef.current || pageRef.current === 0 || items.length >= total) return
    const generation = generationRef.current
    const nextPage = pageRef.current + 1
    loadingRef.current = true
    setLoading(true)
    try {
      const result = await fetcher(nextPage, pageSize, keywordRef.current)
      if (generation !== generationRef.current) return
      setItems((current) => [...current, ...(result.list ?? [])])
      setTotal(result.total ?? 0)
      pageRef.current = nextPage
    } catch {
      // 请求层已提示服务端消息；保留已追加的旧列表
    } finally {
      if (generation === generationRef.current) {
        loadingRef.current = false
        setLoading(false)
      }
    }
  }, [fetcher, pageSize, items.length, total])

  useEffect(() => {
    void reload()
  }, [reload])

  return { items, total, loading, reload, loadMore }
}
