import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type RedisInfo = Schema<'RedisInfo'>;

export function getRedisInfo() {
  return typedApi.get('/api/v1/monitor/redis');
}
