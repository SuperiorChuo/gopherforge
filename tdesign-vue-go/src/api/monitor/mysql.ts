import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type MySQLInfo = Schema<'MySQLInfo'>;

export function getMySQLInfo() {
  return typedApi.get('/api/v1/monitor/mysql');
}
