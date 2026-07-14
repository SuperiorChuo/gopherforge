import request from '@/utils/request'
import type { PageRequest, PageResponse, Menu } from '@/types'

type MenuListParams = PageRequest & { keyword?: string; status?: number }
type MenuCreateData = Omit<Menu, 'id' | 'created_at' | 'children'>
type MenuUpdateData = Partial<MenuCreateData>

export const getMenuList = (params: MenuListParams) =>
  request.get<unknown, PageResponse<Menu>>('/api/v1/menus', { params })

export const getMenuTree = () =>
  request.get<unknown, Menu[]>('/api/v1/menus/tree')

export const createMenu = (data: MenuCreateData) =>
  request.post<unknown, Menu>('/api/v1/menus', data)

export const updateMenu = (id: number, data: MenuUpdateData) =>
  request.put<unknown, Menu>(`/api/v1/menus/${id}`, data)

export const deleteMenu = (id: number) =>
  request.delete<unknown, void>(`/api/v1/menus/${id}`)
