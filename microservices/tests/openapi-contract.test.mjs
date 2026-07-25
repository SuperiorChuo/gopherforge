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
  assert.ok(spec.paths['/api/v1/monitor/mysql']);
  assert.ok(spec.paths['/api/v1/monitor/redis']);
  assert.ok(spec.paths['/api/v1/monitor/jobs']);
  assert.ok(spec.paths['/api/v1/monitor/jobs/health']);
  assert.ok(spec.paths['/api/v1/monitor/jobs/heartbeats']);
  assert.ok(spec.paths['/api/v1/monitor/services']);
  assert.ok(spec.paths['/api/v1/monitor/jobs/{id}/run']);
  assert.ok(spec.paths['/api/v1/monitor/job-logs/cleanup']);
  assert.ok(spec.paths['/api/v1/metrics']);
});

test('monitor OpenAPI exposes concrete heartbeat, services, and unavailable contracts', async () => {
  const raw = await readFile(new URL('../services/monitor/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);
  const schemas = spec.components?.schemas;

  assert.deepEqual(schemas.JobHeartbeatsResponse.required.sort(), ['list', 'total']);
  assert.equal(schemas.JobHeartbeatsResponse.properties.list.type, 'array');
  assert.equal(schemas.JobHeartbeatsResponse.properties.list.items.$ref, '#/components/schemas/JobHeartbeat');
  assert.equal(schemas.JobHeartbeatsResponse.properties.total.type, 'integer');

  assert.deepEqual(schemas.ServiceHealthRow.required.sort(), ['http_code', 'latency_ms', 'name', 'ok']);
  assert.equal(schemas.ServiceHealthRow.properties.name.type, 'string');
  assert.equal(schemas.ServiceHealthRow.properties.ok.type, 'boolean');
  assert.equal(schemas.ServiceHealthRow.properties.http_code.type, 'integer');
  assert.equal(schemas.ServiceHealthRow.properties.latency_ms.type, 'integer');
  assert.equal(schemas.ServiceHealthRow.properties.error.type, 'string');
  assert.equal(schemas.ServicesHealthResponse.properties.list.items.$ref, '#/components/schemas/ServiceHealthRow');
  assert.equal(schemas.ServicesHealthResponse.properties.total.type, 'integer');
  assert.equal(schemas.ServicesHealthResponse.properties.healthy.type, 'integer');
  assert.equal(schemas.ServicesHealthResponse.properties.checked_at.format, 'date-time');

  assert.equal(
    schemas.JobHeartbeatsEnvelope.properties.data.$ref,
    '#/components/schemas/JobHeartbeatsResponse',
  );
  assert.equal(
    schemas.ServicesHealthEnvelope.properties.data.$ref,
    '#/components/schemas/ServicesHealthResponse',
  );

  const heartbeatOperation = spec.paths['/api/v1/monitor/jobs/heartbeats'].get;
  assert.equal(
    heartbeatOperation.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/JobHeartbeatsEnvelope',
  );
  assert.equal(
    heartbeatOperation.responses['503'].content['application/json'].schema.$ref,
    '#/components/schemas/ApiResponse',
  );
  assert.equal(
    spec.paths['/api/v1/monitor/services'].get.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/ServicesHealthEnvelope',
  );
  assert.equal(
    spec.paths['/api/v1/monitor/mysql'].get.responses['503'].content['application/json'].schema.$ref,
    '#/components/schemas/ApiResponse',
  );
});

test('generated web types contain concrete monitor operations and schemas', async () => {
  const types = await readFile(new URL('../web/src/api/generated/schema.d.ts', import.meta.url), 'utf8');

  assert.match(types, /export interface paths/);
  assert.match(types, /"\/api\/v1\/monitor\/jobs\/heartbeats": \{\s+get: operations\["getApiV1MonitorJobsHeartbeats"\];/);
  assert.match(types, /"\/api\/v1\/monitor\/services": \{\s+get: operations\["getApiV1MonitorServices"\];/);
  assert.match(types, /JobHeartbeat: \{[\s\S]*job_key: string;[\s\S]*stale: boolean;/);
  assert.match(types, /ServiceHealthRow: \{[\s\S]*http_code: number;/);
  assert.match(types, /ServiceHealthRow: \{[\s\S]*error\?: string;/);
  assert.match(types, /JobHeartbeatsResponse: \{[\s\S]*list: components\["schemas"\]\["JobHeartbeat"\]\[\];[\s\S]*total: number;/);
  assert.match(types, /ServicesHealthResponse: \{[\s\S]*healthy: number;/);
  assert.match(types, /ServicesHealthResponse: \{[\s\S]*checked_at: string;/);

  const cleanupStart = types.indexOf('  "postApiV1MonitorJobLogsCleanup": {');
  assert.notEqual(cleanupStart, -1);
  const cleanupOperation = types.slice(cleanupStart, types.indexOf('\n  };', cleanupStart));
  assert.match(cleanupOperation, /requestBody\?: \{/);

  const heartbeatStart = types.indexOf('  "getApiV1MonitorJobsHeartbeats": {');
  assert.notEqual(heartbeatStart, -1);
  const heartbeatOperation = types.slice(heartbeatStart, types.indexOf('\n  };', heartbeatStart));
  assert.match(heartbeatOperation, /"200"[\s\S]*JobHeartbeatsEnvelope/);
  assert.match(heartbeatOperation, /"503"[\s\S]*ApiResponse/);
});

test('monitor OpenAPI documents authorization, not-found, and validated job inputs', async () => {
  const raw = await readFile(new URL('../services/monitor/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);
  const apiResponse = spec.components.schemas.ApiResponse;

  assert.deepEqual(apiResponse.required.sort(), ['code', 'message']);

  for (const [path, method] of [
    ['/api/v1/monitor/server', 'get'],
    ['/api/v1/monitor/services', 'get'],
    ['/api/v1/monitor/mysql', 'get'],
    ['/api/v1/monitor/redis', 'get'],
    ['/api/v1/monitor/jobs', 'get'],
    ['/api/v1/monitor/jobs', 'post'],
    ['/api/v1/monitor/jobs/health', 'get'],
    ['/api/v1/monitor/jobs/heartbeats', 'get'],
    ['/api/v1/monitor/jobs/{id}', 'put'],
    ['/api/v1/monitor/jobs/{id}', 'delete'],
    ['/api/v1/monitor/jobs/{id}/start', 'post'],
    ['/api/v1/monitor/jobs/{id}/stop', 'post'],
    ['/api/v1/monitor/jobs/{id}/run', 'post'],
    ['/api/v1/monitor/job-logs/cleanup', 'post'],
  ]) {
    assert.equal(
      spec.paths[path][method].responses['403'].content['application/json'].schema.$ref,
      '#/components/schemas/ApiResponse',
    );
  }

  for (const [path, method] of [
    ['/api/v1/monitor/jobs/{id}', 'put'],
    ['/api/v1/monitor/jobs/{id}', 'delete'],
    ['/api/v1/monitor/jobs/{id}/start', 'post'],
    ['/api/v1/monitor/jobs/{id}/stop', 'post'],
    ['/api/v1/monitor/jobs/{id}/run', 'post'],
  ]) {
    assert.equal(
      spec.paths[path][method].responses['404'].content['application/json'].schema.$ref,
      '#/components/schemas/ApiResponse',
    );
  }

  const cleanup = spec.paths['/api/v1/monitor/job-logs/cleanup'].post;
  assert.equal(cleanup.requestBody.required, false);
  assert.deepEqual(
    cleanup.parameters.find((parameter) => parameter.in === 'query' && parameter.name === 'retention_days').schema,
    { type: 'integer', format: 'int64', minimum: 1 },
  );
  assert.equal(spec.components.schemas.JobLogCleanupRequest.properties.retention_days.minimum, 1);

  const jobList = spec.paths['/api/v1/monitor/jobs'].get;
  assert.deepEqual(jobList.parameters.find((parameter) => parameter.in === 'query' && parameter.name === 'status').schema.enum, [0, 1]);
  const jobHealth = spec.paths['/api/v1/monitor/jobs/health'].get;
  assert.equal(jobHealth.parameters.find((parameter) => parameter.in === 'query' && parameter.name === 'window_hours').schema.minimum, 1);
});
