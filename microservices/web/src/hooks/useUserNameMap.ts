import { useEffect, useState } from 'react'
import request from '@/utils/request'
import type { PageResponse, SystemUser } from '@/types'

// BPM 时间线/待办中心按 §4.4 M1 约定用现有用户接口自行映射操作人姓名。
// 模块级缓存一次拉取结果，多个组件（展开行、抽屉）共享，避免重复请求；
// silent 请求：审批人可能没有用户管理权限，403 时静默降级为「用户 #id」。

let cache: Promise<Record<number, string>> | null = null

function fetchUserNameMap(): Promise<Record<number, string>> {
  if (!cache) {
    cache = request
      .get<unknown, PageResponse<SystemUser>>('/api/v1/users', {
        params: { page: 1, page_size: 500 },
        silent: true,
      })
      .then((res) => {
        const map: Record<number, string> = {}
        for (const u of res?.list ?? []) {
          map[u.id] = u.nickname || u.username
        }
        return map
      })
      .catch(() => {
        cache = null // 失败不缓存，下次挂载重试
        return {}
      })
  }
  return cache
}

/** 用户 id → 昵称/用户名 映射；未加载完成或无权限时返回空映射 */
export function useUserNameMap(): Record<number, string> {
  const [map, setMap] = useState<Record<number, string>>({})
  useEffect(() => {
    let alive = true
    void fetchUserNameMap().then((m) => {
      if (alive) setMap(m)
    })
    return () => {
      alive = false
    }
  }, [])
  return map
}

/** 统一的用户显示名兜底 */
export function displayUserName(map: Record<number, string>, id?: number): string {
  if (id === undefined || id === null) return '-'
  if (id === 0) return '系统'
  return map[id] || `用户 #${id}`
}
