import { useCallback, useState } from 'react'

export type PaginationState = {
  page: number
  page_size: number
}

export type UsePaginationOptions = PaginationState

/** 统一列表分页状态，避免各页面重复处理当前页与每页数量。 */
export function usePagination({ page, page_size }: UsePaginationOptions = { page: 1, page_size: 10 }) {
  const [pagination, setPagination] = useState<PaginationState>({ page, page_size })

  const setPage = useCallback((nextPage: number, nextPageSize = pagination.page_size) => {
    setPagination({ page: nextPage, page_size: nextPageSize })
  }, [pagination.page_size])

  const resetPagination = useCallback(() => {
    setPagination({ page: 1, page_size })
  }, [page_size])

  const updatePagination = useCallback((next: Partial<PaginationState>) => {
    setPagination((current) => ({ ...current, ...next }))
  }, [])

  return { ...pagination, setPage, resetPagination, updatePagination }
}
