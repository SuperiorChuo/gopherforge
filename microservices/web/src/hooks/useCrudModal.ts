import { useCallback, useState } from 'react'

/** 统一新增/编辑实体弹窗状态，表单值由调用方负责填充。 */
export function useCrudModal<T>() {
  const [open, setOpen] = useState(false)
  const [record, setRecord] = useState<T | null>(null)

  const openCreate = useCallback(() => {
    setRecord(null)
    setOpen(true)
  }, [])

  const openEdit = useCallback((next: T) => {
    setRecord(next)
    setOpen(true)
  }, [])

  const close = useCallback(() => {
    setOpen(false)
  }, [])

  return {
    open,
    record,
    openCreate,
    openEdit,
    close,
  }
}
