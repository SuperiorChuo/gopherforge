import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import assert from 'node:assert/strict';

test('generated OpenAPI contract exposes key backend routes', async () => {
  const raw = await readFile(new URL('../server/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  assert.equal(spec.openapi, '3.1.0');
  assert.ok(spec.paths['/api/v1/login']?.post);
  assert.ok(spec.paths['/api/v1/user/me']?.get);
  assert.ok(spec.paths['/api/v1/roles/{id}']?.get);
  assert.ok(spec.components.securitySchemes.BearerAuth);
  assert.deepEqual(spec.paths['/api/v1/login'].post.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/user/me'].get.security, [{ BearerAuth: [] }]);
});

test('generated OpenAPI contract includes typed core schemas', async () => {
  const raw = await readFile(new URL('../server/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  assert.equal(
    spec.paths['/api/v1/login'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/LoginRequest',
  );
  assert.equal(
    spec.paths['/api/v1/login'].post.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/LoginResponseEnvelope',
  );
  assert.equal(
    spec.paths['/api/v1/users/{id}/roles'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/AssignRolesRequest',
  );
  assert.equal(spec.components.schemas.LoginRequest.properties.username.type, 'string');
  assert.deepEqual(spec.components.schemas.LoginRequest.required, ['username', 'password', 'captcha_id', 'captcha_code']);
  assert.equal(spec.components.schemas.ApiResponse.properties.error_code.type, 'string');
  assert.ok(spec.components.schemas.UserInfo.properties.roles.items.$ref.endsWith('/RoleInfo'));
  assert.ok(spec.components.schemas.MenuItem.properties.children.items.$ref.endsWith('/MenuItem'));
});

test('generated frontend OpenAPI types include key paths', async () => {
  const types = await readFile(new URL('../tdesign-vue-go/src/api/generated/schema.d.ts', import.meta.url), 'utf8');

  assert.ok(types.includes('"/api/v1/login"'));
  assert.ok(types.includes('"/api/v1/user/me"'));
  assert.ok(types.includes('"/api/v1/roles/{id}"'));
  assert.ok(types.includes('BearerAuth'));
  assert.ok(types.includes('LoginRequest: {'));
  assert.ok(types.includes('LoginResponseEnvelope:'));
  assert.ok(types.includes('error_code?: string;'));
  assert.ok(types.includes('username: string;'));
  assert.ok(types.includes('data: components["schemas"]["LoginResponse"];'));
});

test('typed API client source is generated-schema aware', async () => {
  const client = await readFile(new URL('../tdesign-vue-go/src/api/generated/client.ts', import.meta.url), 'utf8');

  assert.ok(client.includes('export const typedApi'));
  assert.ok(client.includes('buildApiPath'));
  assert.ok(client.includes('ResponseData'));
});

test('remaining console modules expose typed schemas', async () => {
  const raw = await readFile(new URL('../server/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  const checks = [
    ['/api/v1/departments', 'get', 'DepartmentListEnvelope'],
    ['/api/v1/departments', 'post', 'CreateDepartmentRequest'],
    ['/api/v1/permissions', 'post', 'CreatePermissionRequest'],
    ['/api/v1/dict-types', 'post', 'CreateDictTypeRequest'],
    ['/api/v1/dict-items', 'post', 'CreateDictItemRequest'],
    ['/api/v1/notices/{id}/status', 'put', 'UpdateNoticeStatusRequest'],
    ['/api/v1/operation-logs', 'get', 'OperationLogListEnvelope'],
    ['/api/v1/login-logs/trend', 'get', 'LoginTrendEnvelope'],
    ['/api/v1/online-users/{token_id}', 'delete', 'EmptyEnvelope'],
    ['/api/v1/monitor/server', 'get', 'ServerInfoEnvelope'],
    ['/api/v1/monitor/mysql', 'get', 'MySQLInfoEnvelope'],
    ['/api/v1/monitor/redis', 'get', 'RedisInfoEnvelope'],
  ];

  for (const [path, method, schemaName] of checks) {
    const operation = spec.paths[path]?.[method];
    assert.ok(operation, `${method.toUpperCase()} ${path} is missing`);
    const responseRef = operation.responses?.['200']?.content?.['application/json']?.schema?.$ref;
    const requestRef = operation.requestBody?.content?.['application/json']?.schema?.$ref;
    assert.ok(
      responseRef?.endsWith(`/${schemaName}`) || requestRef?.endsWith(`/${schemaName}`),
      `${method.toUpperCase()} ${path} should reference ${schemaName}`,
    );
  }

  assert.ok(spec.components.schemas.DepartmentItem.properties.children.items.$ref.endsWith('/DepartmentItem'));
  assert.ok(spec.components.schemas.PermissionItem.properties.children.items.$ref.endsWith('/PermissionItem'));
  assert.equal(spec.components.schemas.PermissionItem.properties.description.type, 'string');
  assert.equal(spec.components.schemas.CreatePermissionRequest.properties.description.type, 'string');
  assert.equal(spec.components.schemas.UpdatePermissionRequest.properties.description.type, 'string');
  assert.ok(spec.components.schemas.RedisInfo.properties.keyspace.$ref.endsWith('/RedisKeyspaceInfo'));
});
