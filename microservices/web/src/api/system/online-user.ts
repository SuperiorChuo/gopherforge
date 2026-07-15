import request from '@/utils/request'
import type { OnlineUser } from '@/types'

// 后端返回 { list, total }，条目以 token_id 标识会话
export const getOnlineUserList = () =>
  request
    .get<unknown, { list: OnlineUser[]; total: number }>('/api/v1/online-users')
    .then((res) => res.list ?? [])

export const kickUser = (token_id: string) =>
  request.delete<unknown, void>(`/api/v1/online-users/${token_id}`)

// 仪表盘可选模块：无权限时静默降级
export const getOnlineUserCount = () =>
  request
    .get<unknown, { count: number }>('/api/v1/online-users/count', { silent: true })
    .then((res) => res.count ?? 0)
