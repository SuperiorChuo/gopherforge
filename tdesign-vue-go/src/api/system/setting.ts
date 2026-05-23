import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type SystemSettingItem = Schema<'SystemSettingItem'>;
export type UpsertSystemSettingRequest = Schema<'UpsertSystemSettingRequest'>;
export type BatchUpsertSystemSettingsRequest = Schema<'BatchUpsertSystemSettingsRequest'>;

export function getSystemSettings(group?: string) {
  return typedApi.get('/api/v1/system-settings', {
    query: group ? { group } : undefined,
  });
}

export function getSystemSetting(key: string) {
  return typedApi.get('/api/v1/system-settings/{key}', {
    path: { key },
  });
}

export function updateSystemSetting(key: string, data: UpsertSystemSettingRequest) {
  return typedApi.put('/api/v1/system-settings/{key}', {
    path: { key },
    body: data,
  });
}

export function deleteSystemSetting(key: string) {
  return typedApi.delete('/api/v1/system-settings/{key}', {
    path: { key },
  });
}

export function batchUpdateSystemSettings(data: BatchUpsertSystemSettingsRequest) {
  return typedApi.post('/api/v1/system-settings/batch', {
    body: data,
  });
}
