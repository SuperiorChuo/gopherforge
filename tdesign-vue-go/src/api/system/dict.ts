import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type DictTypeItem = Schema<'DictTypeItem'>;
export type DictType = DictTypeItem;
export type DictItem = Schema<'DictItem'>;
export type DictData = Schema<'DictData'>;
export type DictTypeListResponse = Schema<'DictTypeListResponse'>;
export type DictItemListResponse = Schema<'DictItemListResponse'>;
export type CreateDictTypeRequest = Schema<'CreateDictTypeRequest'>;
export type UpdateDictTypeRequest = Schema<'UpdateDictTypeRequest'>;
export type CreateDictItemRequest = Schema<'CreateDictItemRequest'>;
export type UpdateDictItemRequest = Schema<'UpdateDictItemRequest'>;

export interface DictTypeListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export interface DictItemListRequest extends DictTypeListRequest {
  type_id?: number;
}

export function getDictTypeList(params?: DictTypeListRequest) {
  return typedApi.get('/api/v1/dict-types', {
    query: params,
  });
}

export function getAllDictTypes() {
  return typedApi.get('/api/v1/dict-types/all');
}

export function getDictType(id: number) {
  return typedApi.get('/api/v1/dict-types/{id}', {
    path: { id },
  });
}

export function getItemsByTypeID(id: number) {
  return typedApi.get('/api/v1/dict-types/{id}/items', {
    path: { id },
  });
}

export function createDictType(data: CreateDictTypeRequest) {
  return typedApi.post('/api/v1/dict-types', {
    body: data,
  });
}

export function updateDictType(id: number, data: UpdateDictTypeRequest) {
  return typedApi.put('/api/v1/dict-types/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteDictType(id: number) {
  return typedApi.delete('/api/v1/dict-types/{id}', {
    path: { id },
  });
}

export function getDictItemList(params?: DictItemListRequest) {
  return typedApi.get('/api/v1/dict-items', {
    query: params,
  });
}

export function getDictItem(id: number) {
  return typedApi.get('/api/v1/dict-items/{id}', {
    path: { id },
  });
}

export function createDictItem(data: CreateDictItemRequest) {
  return typedApi.post('/api/v1/dict-items', {
    body: data,
  });
}

export function updateDictItem(id: number, data: UpdateDictItemRequest) {
  return typedApi.put('/api/v1/dict-items/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteDictItem(id: number) {
  return typedApi.delete('/api/v1/dict-items/{id}', {
    path: { id },
  });
}

export function getDictData(code: string) {
  return typedApi.get('/api/v1/dicts/{code}', {
    path: { code },
  });
}

export function getMultipleDictData(codes: string[]) {
  return typedApi.get('/api/v1/dicts', {
    query: { codes: codes.join(',') },
  });
}

export function getAllDictData() {
  return typedApi.get('/api/v1/dicts/all');
}
