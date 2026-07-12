import request from '@/utils/request'
import type { PageRequest, PageResponse, LoginLog, OperationLog } from '@/types'

type LoginLogListParams = PageRequest & { username?: string; status?: number; start_time?: string; end_time?: string }
type OperationLogListParams = PageRequest & { username?: string; module?: string; status?: number; start_time?: string; end_time?: string }

export const getLoginLogList = (params: LoginLogListParams) =>
  request.get<unknown, PageResponse<LoginLog>>('/api/v1/system/login-logs', { params })

export const getOperationLogList = (params: OperationLogListParams) =>
  request.get<unknown, PageResponse<OperationLog>>('/api/v1/system/operation-logs', { params })

export const getOperationLogDetail = (id: number) =>
  request.get<unknown, OperationLog>(`/api/v1/system/operation-logs/${id}`)
