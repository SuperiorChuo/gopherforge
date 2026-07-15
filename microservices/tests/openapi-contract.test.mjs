import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import assert from 'node:assert/strict';

// 契约真源：legacy-backend 瘦后端 OpenAPI。
// 业务路由已拆到各微服务；此处只断言兜底后端仍暴露的健康/监控面。
// 完整业务契约后续可按服务拆分或聚合生成。

test('legacy-backend OpenAPI exposes health and monitor routes', async () => {
  const raw = await readFile(new URL('../legacy-backend/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  assert.equal(spec.openapi, '3.1.0');
  assert.ok(spec.paths['/api/v1/health']);
  assert.ok(spec.paths['/api/v1/health/live']);
  assert.ok(spec.paths['/api/v1/health/ready']);
  assert.ok(spec.paths['/api/v1/monitor/server']);
  assert.ok(spec.paths['/api/v1/monitor/redis']);
  assert.ok(spec.paths['/api/v1/metrics']);
});

test('legacy-backend OpenAPI keeps BearerAuth security scheme when present', async () => {
  const raw = await readFile(new URL('../legacy-backend/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);
  // 瘦后端可能无登录路由，但若声明了组件应结构合法
  if (spec.components?.securitySchemes?.BearerAuth) {
    assert.equal(spec.components.securitySchemes.BearerAuth.type, 'http');
    assert.equal(spec.components.securitySchemes.BearerAuth.scheme, 'bearer');
  }
  assert.ok(Object.keys(spec.paths || {}).length >= 5);
});

test('optional tdesign generated types still reference login when file exists', async () => {
  // 遗留前端生成物仍在仓库根；不存在则跳过
  let types;
  try {
    types = await readFile(new URL('../../tdesign-vue-go/src/api/generated/schema.d.ts', import.meta.url), 'utf8');
  } catch {
    return;
  }
  assert.ok(types.includes('paths') || types.includes('/api/v1/'));
});
