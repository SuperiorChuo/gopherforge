import { useCallback, useState } from 'react'

export type ConfirmActionOptions = {
  action: () => Promise<void>
  onSuccess?: () => void
  onError?: () => void
}

/** 统一异步删除/启停动作的执行态与成功/失败回调。确认 UI 仍由 Popconfirm 等领域组件负责。 */
export function useConfirmAction() {
  const [loading, setLoading] = useState(false)

  const run = useCallback(async ({ action, onSuccess, onError }: ConfirmActionOptions) => {
    if (loading) return
    setLoading(true)
    try {
      await action()
      onSuccess?.()
    } catch {
      onError?.()
    } finally {
      setLoading(false)
    }
  }, [loading])

  return { loading, run }
}
