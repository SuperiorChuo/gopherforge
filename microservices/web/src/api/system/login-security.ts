import request from '@/utils/request'

export interface BlockedIPEntry {
  ip: string
  ttl_seconds: number
}

/** 被屏蔽 IP 列表（IP 级失败护盾，auth 服务 Redis） */
export const getBlockedIPs = () =>
  request.get<unknown, { items: BlockedIPEntry[] }>('/api/v1/login-security/blocked-ips')

/** 解封指定 IP */
export const unblockIP = (ip: string) =>
  request.delete<unknown, { ip: string }>(`/api/v1/login-security/blocked-ips/${encodeURIComponent(ip)}`)
