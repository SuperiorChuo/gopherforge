import request from '@/utils/request'
import type { SystemSetting } from '@/types'

export const getSettingList = (group?: string) =>
  request.get<unknown, SystemSetting[]>('/api/v1/system-settings', { params: group ? { group } : undefined })

export const getSetting = (key: string) =>
  request.get<unknown, SystemSetting>(`/api/v1/system-settings/${key}`)

export const upsertSetting = (key: string, value: Record<string, unknown>) =>
  request.put<unknown, SystemSetting>(`/api/v1/system-settings/${key}`, { value_json: value })

export const deleteSetting = (key: string) =>
  request.delete<unknown, void>(`/api/v1/system-settings/${key}`)
