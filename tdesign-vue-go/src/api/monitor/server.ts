import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type ServerInfo = Schema<'ServerInfo'>;

export function getServerInfo() {
  return typedApi.get('/api/v1/monitor/server');
}
