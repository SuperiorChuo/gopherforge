import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import assert from 'node:assert/strict';

// 契约真源：monitor 服务 OpenAPI（健康/监控/指标 + 迁移承载）。
// 业务路由在其它微服务中。

test('monitor OpenAPI exposes health and monitor routes', async () => {
  const raw = await readFile(new URL('../services/monitor/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  assert.equal(spec.openapi, '3.1.0');
  assert.ok(spec.paths['/api/v1/health']);
  assert.ok(spec.paths['/api/v1/health/live']);
  assert.ok(spec.paths['/api/v1/health/ready']);
  assert.ok(spec.paths['/api/v1/monitor/server']);
  assert.ok(spec.paths['/api/v1/monitor/redis']);
  assert.ok(spec.paths['/api/v1/metrics']);
  assert.ok(Object.keys(spec.paths || {}).length >= 5);
});

test('optional web generated types file is schema-shaped when present', async () => {
  let types;
  try {
    types = await readFile(new URL('../web/src/api/generated/schema.d.ts', import.meta.url), 'utf8');
  } catch {
    return;
  }
  assert.ok(types.includes('export interface paths') || types.includes('paths'));
});
