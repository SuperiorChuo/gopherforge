import { useCallback } from 'react'
import { useAppSelector } from '@/hooks/store'

/**
 * 权限判断，规则与后端 PermissionMiddleware 一致：
 * 角色含 super_admin 直接放行，否则查 /user/me 返回的权限码列表。
 */
export function usePermission() {
  // 分字段订阅而不是整片 s.auth：RTK/Immer 下任何一个 auth action（含 loading
  // 翻转）都会换掉 slice 对象引用，订阅整片等于每次都重渲全部消费者——而
  // MainLayout 的菜单树与命令面板都以 hasPerm 为依赖，代价是全量重建。
  const roles = useAppSelector((s) => s.auth.userInfo?.roles)
  const permissions = useAppSelector((s) => s.auth.permissions)
  const isSuperAdmin = roles?.some((r) => r.code === 'super_admin') ?? false

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
