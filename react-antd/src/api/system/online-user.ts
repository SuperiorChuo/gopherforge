import request from '@/utils/request'
import type { OnlineUser } from '@/types'

export const getOnlineUserList = () =>
  request.get<unknown, OnlineUser[]>('/api/v1/system/online-users')

export const kickUser = (session_id: string) =>
  request.delete<unknown, void>(`/api/v1/system/online-users/${session_id}`)
