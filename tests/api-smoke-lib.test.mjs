import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildConfig,
  getJsonPath,
  jsonObject,
  normalizeRunId,
  statusMatches,
} from './api-smoke-lib.mjs';

test('statusMatches supports wildcard and comma-separated HTTP codes', () => {
  assert.equal(statusMatches('*', 503), true);
  assert.equal(statusMatches('200,201', 200), true);
  assert.equal(statusMatches('200,201', 404), false);
});

test('normalizeRunId keeps only portable request id characters', () => {
  assert.equal(normalizeRunId('2026-05-20 smoke#1'), '2026_05_20_smoke_1');
});

test('jsonObject builds JSON with string values', () => {
  assert.equal(jsonObject({ nickname: '管理后台', role: 'admin' }), '{"nickname":"管理后台","role":"admin"}');
});

test('getJsonPath reads nested values and rejects missing paths', () => {
  const data = { code: 200, data: { user: { username: 'admin' } } };

  assert.equal(getJsonPath(data, 'data.user.username'), 'admin');
  assert.throws(() => getJsonPath(data, 'data.user.email'), /missing JSON path/);
});

test('buildConfig reads environment overrides', () => {
  const config = buildConfig({
    API_BASE_URL: 'http://127.0.0.1:8081/api/v1/',
    SMOKE_USERNAME: 'tester',
    SMOKE_PASSWORD: 'secret',
    SMOKE_TIMEOUT: '5',
    SMOKE_RUN_ID: 'run-id',
  });

  assert.deepEqual(config, {
    apiBaseUrl: 'http://127.0.0.1:8081/api/v1',
    username: 'tester',
    password: 'secret',
    timeoutSeconds: 5,
    runId: 'run-id',
    safeRunId: 'run_id',
  });
});
