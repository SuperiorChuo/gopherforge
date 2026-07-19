import { useCallback } from 'react'
import { useAppSelector } from '@/hooks/store'

/**
 * 权限判断，规则与后端 PermissionMiddleware 一致：
 * 角色含 super_admin 直接放行，否则查 /user/me 返回的权限码列表。
 */
export function usePermission() {
  const { userInfo, permissions } = useAppSelector((s) => s.auth)
  const isSuperAdmin = userInfo?.roles?.some((r) => r.code === 'super_admin') ?? false

  const hasPerm = useCallback(
    (code?: string) => {
      if (!code) return true
      if (isSuperAdmin) return true
      return permissions.includes(code)
    },
    [isSuperAdmin, permissions],
  )

  return { hasPerm, isSuperAdmin, permissions }
}
