import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export interface MenuListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export type MenuListResponse = Schema<'MenuListResponse'>;
export type MenuItem = Schema<'MenuItem'>;
export type CreateMenuRequest = Schema<'CreateMenuRequest'>;
export type UpdateMenuRequest = Schema<'UpdateMenuRequest'>;

export function getMenuList(params: MenuListRequest) {
  return typedApi.get('/api/v1/menus', {
    query: params,
  });
}

export function getMenuTree() {
  return typedApi.get('/api/v1/menus/tree');
}

export function getMenu(id: number) {
  return typedApi.get('/api/v1/menus/{id}', {
    path: { id },
  });
}

export function createMenu(data: CreateMenuRequest) {
  return typedApi.post('/api/v1/menus', {
    body: data,
  });
}

export function updateMenu(id: number, data: UpdateMenuRequest) {
  return typedApi.put('/api/v1/menus/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteMenu(id: number) {
  return typedApi.delete('/api/v1/menus/{id}', {
    path: { id },
  });
}
