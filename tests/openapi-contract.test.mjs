import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import assert from 'node:assert/strict';

test('generated OpenAPI contract exposes key backend routes', async () => {
  const raw = await readFile(new URL('../server/docs/openapi.json', import.meta.url), 'utf8');
  const spec = JSON.parse(raw);

  assert.equal(spec.openapi, '3.1.0');
  assert.ok(spec.paths['/api/v1/login']?.post);
  assert.ok(spec.paths['/api/v1/login/2fa/verify']?.post);
  assert.ok(spec.paths['/api/v1/auth/login']?.post);
  assert.ok(spec.paths['/api/v1/auth/login/2fa/verify']?.post);
  assert.ok(spec.paths['/api/v1/ws/notifications']?.get);
  assert.ok(spec.paths['/api/v1/ws/notifications/ticket']?.post);
  assert.ok(spec.paths['/api/v1/oauth/bind']?.post);
  assert.ok(spec.paths['/api/v1/oauth/unbind']?.post);
  assert.ok(spec.paths['/api/v1/user/2fa/recovery-codes']?.post);
  assert.ok(spec.paths['/api/v1/system-settings']?.get);
  assert.ok(spec.paths['/api/v1/system-settings/batch']?.post);
  assert.ok(spec.paths['/api/v1/system-settings/{key}']?.delete);
  assert.ok(spec.paths['/api/v1/user/me']?.get);
  assert.ok(spec.paths['/api/v1/roles/{id}']?.get);
  assert.ok(spec.components.securitySchemes.BearerAuth);
  assert.deepEqual(spec.paths['/api/v1/login'].post.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/login/2fa/verify'].post.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/auth/login'].post.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/auth/login/2fa/verify'].post.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/ws/notifications'].get.security ?? [], []);
  assert.deepEqual(spec.paths['/api/v1/ws/notifications/ticket'].post.security, [{ BearerAuth: [] }]);
  assert.deepEqual(spec.paths['/api/v1/user/2fa/recovery-codes'].post.security, [{ BearerAuth: [] }]);
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
    spec.paths['/api/v1/auth/login'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/ConsoleLoginRequest',
  );
  assert.equal(
    spec.paths['/api/v1/auth/login'].post.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/ConsoleLoginResponseEnvelope',
  );
  assert.equal(
    spec.paths['/api/v1/auth/login/2fa/verify'].post.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/ConsoleSessionEnvelope',
  );
  assert.equal(
    spec.paths['/api/v1/users/{id}/roles'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/AssignRolesRequest',
  );
  assert.equal(
    spec.paths['/api/v1/oauth/bind'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/OAuthBindRequest',
  );
  assert.equal(
    spec.paths['/api/v1/oauth/unbind'].post.requestBody.content['application/json'].schema.$ref,
    '#/components/schemas/OAuthUnbindRequest',
  );
  assert.ok(spec.paths['/api/v1/oauth/bind'].post.responses['409']);
  assert.ok(spec.paths['/api/v1/oauth/bind'].post.responses['503']);
  assert.ok(spec.paths['/api/v1/oauth/unbind'].post.responses['404']);
  assert.ok(spec.paths['/api/v1/oauth/unbind'].post.responses['503']);
  assert.equal(
    spec.paths['/api/v1/files/{id}/download'].get.responses['200'].content['application/octet-stream'].schema.format,
    'binary',
  );
  assert.equal(
    spec.paths['/api/v1/files/{id}/preview'].get.responses['200'].content['application/octet-stream'].schema.format,
    'binary',
  );
  assert.equal(spec.paths['/api/v1/files/{id}/download'].get.responses['200'].content['application/json'], undefined);

  const hashCheck = spec.paths['/api/v1/files/hash/check']?.get;
  assert.ok(hashCheck, 'GET /api/v1/files/hash/check is missing');
  assert.equal(
    hashCheck.responses['200'].content['application/json'].schema.$ref,
    '#/components/schemas/FileHashCheckEnvelope',
  );
  assert.deepEqual(
    hashCheck.parameters.find((param) => param.in === 'query' && param.name === 'hash'),
    { name: 'hash', in: 'query', required: true, schema: { type: 'string' } },
  );

  assert.equal(
    spec.components.schemas.FileHashCheckEnvelope.properties.data.$ref,
    '#/components/schemas/FileHashCheck',
  );
  const fileHashCheck = spec.components.schemas.FileHashCheck;
  assert.equal(fileHashCheck.properties.exists.type, 'boolean');
  assert.equal(fileHashCheck.properties.file.$ref, '#/components/schemas/FileItem');
  assert.deepEqual(fileHashCheck.required, ['exists']);

  const fileStats = spec.components.schemas.FileStats;
  assert.deepEqual(Object.keys(fileStats.properties).sort(), ['by_type', 'total', 'total_size']);
  assert.equal(fileStats.properties.total.type, 'integer');
  assert.equal(fileStats.properties.total_size.type, 'integer');
  assert.equal(fileStats.properties.by_type.type, 'object');
  assert.equal(fileStats.properties.by_type.additionalProperties.$ref, '#/components/schemas/FileTypeStat');

  assert.equal(spec.components.schemas.LoginRequest.properties.username.type, 'string');
  assert.deepEqual(spec.components.schemas.LoginRequest.required, ['username', 'password', 'captcha_id', 'captcha_code']);
  assert.equal(spec.components.schemas.ApiResponse.properties.error_code.type, 'string');
  assert.ok(spec.components.schemas.TOTPRecoveryCodesEnvelope);
  assert.ok(spec.components.schemas.NotificationTicketResponse);
  assert.ok(spec.components.schemas.ConsoleSessionResponse.properties.user.$ref.endsWith('/ConsoleSessionUser'));
  assert.ok(spec.components.schemas.NotificationMessage.properties.created_at);
  assert.equal(spec.paths['/api/v1/ws/notifications'].get.parameters[0].name, 'ticket');
  assert.equal(spec.paths['/api/v1/ws/notifications'].get.parameters[0].required, true);
  assert.equal(spec.paths['/api/v1/ws/notifications'].get['x-websocket'], true);
  assert.ok(spec.paths['/api/v1/ws/notifications'].get.responses['101']);
  assert.equal(spec.paths['/api/v1/ws/notifications'].get.responses['200'], undefined);
  assert.deepEqual(spec.components.schemas.TOTPVerifyRequest.required, ['code', 'current_password']);
  assert.equal(spec.components.schemas.TOTPRecoveryCodesResponse.properties.recovery_codes.items.type, 'string');
  assert.ok(spec.components.schemas.UserInfo.properties.roles.items.$ref.endsWith('/RoleInfo'));
  assert.ok(spec.components.schemas.MenuItem.properties.children.items.$ref.endsWith('/MenuItem'));
});

test('generated frontend OpenAPI types include key paths', async () => {
  const types = await readFile(new URL('../tdesign-vue-go/src/api/generated/schema.d.ts', import.meta.url), 'utf8');

  assert.ok(types.includes('"/api/v1/login"'));
  assert.ok(types.includes('"/api/v1/login/2fa/verify"'));
  assert.ok(types.includes('"/api/v1/auth/login/2fa/verify"'));
  assert.ok(types.includes('"/api/v1/ws/notifications"'));
  assert.ok(types.includes('"/api/v1/ws/notifications/ticket"'));
  assert.ok(types.includes('"/api/v1/oauth/bind"'));
  assert.ok(types.includes('"/api/v1/oauth/unbind"'));
  assert.ok(types.includes('OAuthBindRequest:'));
  assert.ok(types.includes('OAuthUnbindRequest:'));
  assert.ok(types.includes('"/api/v1/user/2fa/recovery-codes"'));
  assert.ok(types.includes('"/api/v1/user/me"'));
  assert.ok(types.includes('"/api/v1/roles/{id}"'));
  assert.ok(types.includes('BearerAuth'));
  assert.ok(types.includes('LoginRequest: {'));
  assert.ok(types.includes('LoginResponseEnvelope:'));
  assert.ok(types.includes('ConsoleSessionEnvelope:'));
  assert.ok(types.includes('NotificationMessageEnvelope:'));
  assert.ok(types.includes('NotificationTicketResponse:'));
  assert.ok(types.includes('TOTPRecoveryCodesResponse:'));
  assert.ok(types.includes('FileHashCheckEnvelope:'));
  assert.ok(types.includes('exists: boolean;'));
  assert.match(types, /"getApiV1FilesHashCheck": \{[\s\S]*?query: \{[\s\S]*?hash: string;[\s\S]*?\};/);
  assert.match(types, /"getApiV1WsNotifications": \{[\s\S]*?query: \{[\s\S]*?ticket: string;[\s\S]*?\};/);
  assert.ok(types.includes('by_type?: Record<string, components["schemas"]["FileTypeStat"]>;'));
  assert.ok(types.includes('error_code?: string;'));
  assert.ok(types.includes('username: string;'));
  assert.ok(types.includes('data: components["schemas"]["LoginResponse"];'));
  assert.ok(types.includes('data: components["schemas"]["ConsoleSessionResponse"];'));
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
