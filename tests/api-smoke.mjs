#!/usr/bin/env node

import { createHash } from 'node:crypto';

import { buildConfig, decodeTextCaptchaCode, getJsonPath, jsonObject, statusMatches } from './api-smoke-lib.mjs';

class SmokeError extends Error {
  constructor(message, context = {}) {
    super(message);
    this.name = 'SmokeError';
    this.context = context;
  }
}

const config = buildConfig();
const state = {
  accessToken: '',
  refreshToken: '',
  roleId: '',
  permissionId: '',
  settingKeys: [],
  fileId: '',
  noticeId: '',
  originalNickname: '',
  profileNeedsRestore: false,
  loggedOut: false,
  lastStep: 'initializing',
  lastResponse: null,
};

function step(name) {
  state.lastStep = name;
  console.log(`[api-smoke] ${name}`);
}

function printFailure(error) {
  console.error('');
  console.error('API smoke failed');
  console.error(`Step: ${state.lastStep}`);
  console.error(`Reason: ${error.message}`);
  console.error(`API_BASE_URL: ${config.apiBaseUrl}`);

  if (state.lastResponse) {
    console.error(`HTTP status: ${state.lastResponse.status}`);
    console.error('Response body:');
    console.error(state.lastResponse.text || '<empty>');
  }

  console.error('');
  console.error('Next checks:');
  console.error('1. Start dependencies and backend: docker compose up -d go-admin-kit-mysql go-admin-kit-redis, then start backend.');
  console.error(`2. Confirm readiness: Invoke-WebRequest ${config.apiBaseUrl}/health/ready`);
  console.error('3. Confirm credentials, or override: SMOKE_USERNAME=... SMOKE_PASSWORD=... npm run smoke:api.');
  console.error('4. If login is locked by repeated failures, wait for the configured login_limit window or clear local Redis.');
}

async function request(method, path, expected, body, token = '') {
  const headers = {
    Accept: 'application/json',
    'X-Request-ID': `api-smoke-${config.safeRunId}`,
  };

  const init = {
    method,
    headers,
    signal: AbortSignal.timeout(config.timeoutSeconds * 1000),
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  if (body !== undefined && body !== '') {
    headers['Content-Type'] = 'application/json';
    init.body = typeof body === 'string' ? body : JSON.stringify(body);
  }

  let response;
  try {
    response = await fetch(`${config.apiBaseUrl}${path}`, init);
  } catch (error) {
    throw new SmokeError(`request failed: ${method} ${path}: ${error.message}`);
  }

  const text = await response.text();
  state.lastResponse = { status: response.status, text };

  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch (error) {
      throw new SmokeError(`invalid JSON response for ${method} ${path}: ${error.message}`);
    }
  }

  if (!statusMatches(expected, response.status)) {
    throw new SmokeError(`unexpected HTTP status for ${method} ${path}; expected ${expected}`);
  }

  return { status: response.status, data, text };
}

async function requestMultipart(path, expected, form, token = '') {
  const headers = {
    Accept: 'application/json',
    'X-Request-ID': `api-smoke-${config.safeRunId}`,
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let response;
  try {
    response = await fetch(`${config.apiBaseUrl}${path}`, {
      method: 'POST',
      headers,
      body: form,
      signal: AbortSignal.timeout(config.timeoutSeconds * 1000),
    });
  } catch (error) {
    throw new SmokeError(`request failed: POST ${path}: ${error.message}`);
  }

  const text = await response.text();
  state.lastResponse = { status: response.status, text };

  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch (error) {
      throw new SmokeError(`invalid JSON response for POST ${path}: ${error.message}`);
    }
  }

  if (!statusMatches(expected, response.status)) {
    throw new SmokeError(`unexpected HTTP status for POST ${path}; expected ${expected}`);
  }

  return { status: response.status, data, text };
}

async function requestBytes(method, path, expected, token = '') {
  const headers = {
    'X-Request-ID': `api-smoke-${config.safeRunId}`,
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let response;
  try {
    response = await fetch(`${config.apiBaseUrl}${path}`, {
      method,
      headers,
      signal: AbortSignal.timeout(config.timeoutSeconds * 1000),
    });
  } catch (error) {
    throw new SmokeError(`request failed: ${method} ${path}: ${error.message}`);
  }

  const contentType = response.headers.get('content-type') || '';
  const bytes = Buffer.from(await response.arrayBuffer());
  const text = contentType.includes('json') || contentType.startsWith('text/')
    ? bytes.toString('utf8')
    : `<${bytes.length} bytes; content-type=${contentType || 'unknown'}>`;
  state.lastResponse = { status: response.status, text };

  if (!statusMatches(expected, response.status)) {
    throw new SmokeError(`unexpected HTTP status for ${method} ${path}; expected ${expected}`);
  }

  return { status: response.status, bytes, headers: response.headers };
}

async function cleanupRequest(method, path, body, token = state.accessToken) {
  try {
    await request(method, path, '*', body, token);
  } catch {
    // Best-effort cleanup should not mask the original smoke result.
  }
}

async function cleanup() {
  if (state.noticeId && state.accessToken) {
    await cleanupRequest('DELETE', `/notices/${state.noticeId}`);
  }

  if (state.fileId && state.accessToken) {
    await cleanupRequest('DELETE', `/files/${state.fileId}`);
  }

  for (const key of state.settingKeys) {
    if (state.accessToken) {
      await cleanupRequest('DELETE', `/system-settings/${encodeURIComponent(key)}`);
    }
  }

  if (state.profileNeedsRestore && state.accessToken) {
    await cleanupRequest('PUT', '/user/profile', jsonObject({ nickname: state.originalNickname }));
  }

  if (state.roleId && state.accessToken) {
    await cleanupRequest('DELETE', `/roles/${state.roleId}`);
  }

  if (state.permissionId && state.accessToken) {
    await cleanupRequest('DELETE', `/permissions/${state.permissionId}`);
  }

  if (!state.loggedOut && state.accessToken) {
    const body = state.refreshToken ? jsonObject({ refresh_token: state.refreshToken }) : '';
    await cleanupRequest('POST', '/logout', body);
  }
}

function assertResponse(condition, description) {
  if (!condition) {
    throw new SmokeError(description);
  }
}

function md5Hex(buffer) {
  return createHash('md5').update(buffer).digest('hex');
}

function hasItemWithId(items, id) {
  return Array.isArray(items) && items.some((item) => String(item?.id) === String(id));
}

const smokePng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADUlEQVR42mP8z8BQDwAFgwJ/lK3g7wAAAABJRU5ErkJggg==',
  'base64',
);

async function main() {
  step('health readiness');
  let response = await request('GET', '/health/ready', 200);
  assertResponse(response.data?.code === 200 && response.data?.data?.status === 'ok', 'readiness did not return code=200 and status=ok');

  step('captcha image fields');
  response = await request('GET', '/captcha', 200);
  const captcha = response.data?.data;
  assertResponse(
    response.data?.code === 200 &&
      captcha?.type === 'text' &&
      typeof captcha.key === 'string' &&
      typeof captcha.image === 'string' &&
      captcha.code_hint === undefined &&
      Number.isFinite(captcha.width) &&
      Number.isFinite(captcha.height),
    'captcha response is missing expected text captcha fields',
  );
  const captchaCode = decodeTextCaptchaCode(captcha.image);

  step('captcha verify');
  response = await request('POST', '/captcha/verify', 200, jsonObject({ key: captcha.key, code: captchaCode }));
  assertResponse(response.data?.code === 200, 'captcha verification did not succeed');

  step('login');
  response = await request(
    'POST',
    '/login',
    200,
    jsonObject({
      username: config.username,
      password: config.password,
      captcha_id: captcha.key,
      captcha_code: captchaCode,
    }),
  );
  assertResponse(
    response.data?.code === 200 &&
      typeof response.data?.data?.access_token === 'string' &&
      typeof response.data?.data?.refresh_token === 'string' &&
      response.data?.data?.user?.username === config.username,
    'login response is missing tokens or user',
  );
  state.accessToken = getJsonPath(response.data, 'data.access_token');
  state.refreshToken = getJsonPath(response.data, 'data.refresh_token');

  step('user/me');
  response = await request('GET', '/user/me', 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.username === config.username &&
      Array.isArray(response.data?.data?.permissions),
    'user/me did not return the logged-in user',
  );
  state.originalNickname = getJsonPath(response.data, 'data.nickname');

  step('user menus');
  response = await request('GET', '/user/menus', 200, '', state.accessToken);
  assertResponse(response.data?.code === 200 && Array.isArray(response.data?.data) && response.data.data.length > 0, 'user/menus did not return a non-empty menu array');

  step('refresh token rotation');
  const oldRefreshToken = state.refreshToken;
  response = await request('POST', '/refresh', 200, jsonObject({ refresh_token: oldRefreshToken }));
  assertResponse(
    response.data?.code === 200 &&
      typeof response.data?.data?.access_token === 'string' &&
      typeof response.data?.data?.refresh_token === 'string' &&
      response.data.data.refresh_token !== oldRefreshToken,
    'refresh did not rotate refresh_token',
  );
  state.accessToken = getJsonPath(response.data, 'data.access_token');
  state.refreshToken = getJsonPath(response.data, 'data.refresh_token');

  step('old refresh token rejected');
  response = await request('POST', '/refresh', 401, jsonObject({ refresh_token: oldRefreshToken }));
  assertResponse(response.data?.code === 401, 'old refresh token was not rejected after rotation');

  step('profile update');
  const newNickname = `api-smoke-${config.safeRunId}`;
  response = await request('PUT', '/user/profile', 200, jsonObject({ nickname: newNickname }), state.accessToken);
  state.profileNeedsRestore = true;
  assertResponse(response.data?.code === 200 && response.data?.data?.nickname === newNickname, 'profile nickname did not update');

  step('profile restore');
  response = await request('PUT', '/user/profile', 200, jsonObject({ nickname: state.originalNickname }), state.accessToken);
  assertResponse(response.data?.code === 200 && response.data?.data?.nickname === state.originalNickname, 'profile nickname did not restore');
  state.profileNeedsRestore = false;

  step('system settings upsert');
  const settingsGroup = `smoke_${config.safeRunId.toLowerCase()}`;
  const primarySettingKey = `${settingsGroup}.primary`;
  const secondarySettingKey = `${settingsGroup}.secondary`;
  state.settingKeys.push(primarySettingKey, secondarySettingKey);
  response = await request(
    'PUT',
    `/system-settings/${primarySettingKey}`,
    200,
    jsonObject({
      value_json: {
        label: `API Smoke ${config.safeRunId}`,
        enabled: true,
      },
    }),
    state.accessToken,
  );
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.setting_key === primarySettingKey &&
      response.data?.data?.value_json?.enabled === true,
    'system setting upsert did not persist value_json',
  );

  step('system settings batch and list');
  response = await request(
    'POST',
    '/system-settings/batch',
    200,
    jsonObject({
      settings: [
        { setting_key: primarySettingKey, value_json: { label: `API Smoke ${config.safeRunId}`, enabled: false } },
        { setting_key: secondarySettingKey, value_json: { count: 2, mode: 'batch' } },
      ],
    }),
    state.accessToken,
  );
  assertResponse(
    response.data?.code === 200 &&
      Array.isArray(response.data?.data) &&
      response.data.data.length === 2 &&
      response.data.data.some((item) => item.setting_key === primarySettingKey && item.value_json?.enabled === false) &&
      response.data.data.some((item) => item.setting_key === secondarySettingKey && item.value_json?.mode === 'batch'),
    'system settings batch upsert did not return both settings',
  );

  response = await request('GET', `/system-settings?group=${settingsGroup}`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      Array.isArray(response.data?.data) &&
      response.data.data.length >= 2 &&
      response.data.data.every((item) => String(item.setting_key).startsWith(`${settingsGroup}.`)),
    'system settings list did not return the smoke group',
  );

  step('system settings cleanup');
  for (const key of [...state.settingKeys]) {
    response = await request('DELETE', `/system-settings/${key}`, 200, '', state.accessToken);
    assertResponse(response.data?.code === 200, `system setting delete failed for ${key}`);
  }
  state.settingKeys = [];

  step('file upload');
  const fileName = `api-smoke-${config.safeRunId}.png`;
  const expectedFileHash = md5Hex(smokePng);
  const form = new FormData();
  form.set('file', new Blob([smokePng], { type: 'image/png' }), fileName);
  response = await requestMultipart('/files/upload', 200, form, state.accessToken);
  state.fileId = getJsonPath(response.data, 'data.id');
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.file_name === fileName &&
      response.data?.data?.file_type === 'image' &&
      response.data?.data?.mime_type === 'image/png' &&
      response.data?.data?.hash === expectedFileHash,
    'file upload did not return expected image metadata',
  );

  step('file hash and detail');
  response = await request('GET', `/files/hash/check?hash=${expectedFileHash}`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.exists === true &&
      String(response.data?.data?.file?.id) === state.fileId,
    'file hash check did not find uploaded file',
  );

  response = await request('GET', `/files/${state.fileId}`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      String(response.data?.data?.id) === state.fileId &&
      response.data?.data?.hash === expectedFileHash,
    'file detail did not return uploaded file metadata',
  );

  step('file preview and download');
  let binaryResponse = await requestBytes('GET', `/files/${state.fileId}/preview`, 200, state.accessToken);
  assertResponse(
    binaryResponse.headers.get('content-type') === 'image/png' && binaryResponse.bytes.equals(smokePng),
    'file preview did not stream the uploaded image',
  );

  binaryResponse = await requestBytes('GET', `/files/${state.fileId}/download`, 200, state.accessToken);
  assertResponse(
    binaryResponse.headers.get('content-type') === 'image/png' &&
      binaryResponse.headers.get('content-disposition')?.includes(fileName) &&
      binaryResponse.bytes.equals(smokePng),
    'file download did not stream the uploaded image attachment',
  );

  step('file cleanup');
  response = await request('DELETE', `/files/${state.fileId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200, 'file delete did not succeed');
  state.fileId = '';

  response = await request('GET', `/files/hash/check?hash=${expectedFileHash}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200 && response.data?.data?.exists === false, 'file hash check still found deleted file');

  step('notice create inactive');
  const noticeTitle = `API Smoke Notice ${config.safeRunId}`;
  response = await request(
    'POST',
    '/notices',
    200,
    jsonObject({
      title: noticeTitle,
      content: `created by tests/api-smoke.mjs ${config.safeRunId}`,
      type: 1,
      status: 2,
    }),
    state.accessToken,
  );
  state.noticeId = getJsonPath(response.data, 'data.id');
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.title === noticeTitle &&
      response.data?.data?.status === 2,
    'notice create did not return inactive notice',
  );

  step('notice list and activate');
  response = await request('GET', `/notices?keyword=${encodeURIComponent(noticeTitle)}&status=2`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      hasItemWithId(response.data?.data?.list, state.noticeId),
    'notice list did not include created inactive notice',
  );

  response = await request('PUT', `/notices/${state.noticeId}/status`, 200, jsonObject({ status: 1 }), state.accessToken);
  assertResponse(response.data?.code === 200, 'notice status update did not succeed');

  response = await request('GET', '/notices/active?type=1', 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      hasItemWithId(response.data?.data, state.noticeId),
    'active notices did not include activated notice',
  );

  step('notice cleanup');
  response = await request('DELETE', `/notices/${state.noticeId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200, 'notice delete did not succeed');
  state.noticeId = '';

  step('permission description create');
  const permissionCode = `smoke.permission.${config.safeRunId}`;
  const permissionInitialDescription = `created by api smoke ${config.safeRunId}`;
  response = await request(
    'POST',
    '/permissions',
    200,
    jsonObject({
      name: `API Smoke Permission ${config.safeRunId}`,
      code: permissionCode,
      description: permissionInitialDescription,
      type: 2,
      path: `/api/v1/smoke/${config.safeRunId}`,
      method: 'GET',
      parent_id: 2,
    }),
    state.accessToken,
  );
  state.permissionId = getJsonPath(response.data, 'data.id');
  assertResponse(
    response.data?.code === 200 &&
      response.data?.data?.code === permissionCode &&
      response.data?.data?.description === permissionInitialDescription,
    'permission create did not persist description',
  );

  step('permission description detail');
  response = await request('GET', `/permissions/${state.permissionId}`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 &&
      String(response.data?.data?.id) === state.permissionId &&
      response.data?.data?.description === permissionInitialDescription,
    'permission detail did not return description',
  );

  step('permission description update');
  const permissionUpdatedDescription = `updated by api smoke ${config.safeRunId}`;
  response = await request('PUT', `/permissions/${state.permissionId}`, 200, jsonObject({ description: permissionUpdatedDescription }), state.accessToken);
  assertResponse(response.data?.code === 200 && response.data?.data?.description === permissionUpdatedDescription, 'permission update did not change description');

  step('permission description clear');
  response = await request('PUT', `/permissions/${state.permissionId}`, 200, jsonObject({ description: '' }), state.accessToken);
  assertResponse(response.data?.code === 200 && response.data?.data?.description === '', 'permission update did not clear description');

  step('permission cleanup');
  response = await request('DELETE', `/permissions/${state.permissionId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200, 'permission delete did not succeed');
  state.permissionId = '';

  step('role data_scope create');
  const roleCode = `smoke_api_${config.safeRunId}`;
  response = await request(
    'POST',
    '/roles',
    200,
    jsonObject({
      name: `API Smoke ${config.safeRunId}`,
      code: roleCode,
      description: 'created by tests/api-smoke.mjs',
      data_scope: 'self',
    }),
    state.accessToken,
  );
  state.roleId = getJsonPath(response.data, 'data.id');
  assertResponse(response.data?.code === 200 && response.data?.data?.code === roleCode && response.data?.data?.data_scope === 'self', 'role create did not return the expected data_scope');

  step('role data_scope detail');
  response = await request('GET', `/roles/${state.roleId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200 && String(response.data?.data?.id) === state.roleId && response.data?.data?.data_scope === 'self', 'role detail did not preserve data_scope');

  step('role cleanup');
  response = await request('DELETE', `/roles/${state.roleId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200, 'role delete did not succeed');
  state.roleId = '';

  step('logout');
  response = await request('POST', '/logout', 200, jsonObject({ refresh_token: state.refreshToken }), state.accessToken);
  assertResponse(response.data?.code === 200, 'logout did not succeed');
  state.loggedOut = true;

  step('logout invalidates access token');
  response = await request('GET', '/user/me', 401, '', state.accessToken);
  assertResponse(response.data?.code === 401, 'logged-out access token did not return 401');

  console.log(`[api-smoke] ok: API smoke completed against ${config.apiBaseUrl}`);
}

try {
  await main();
} catch (error) {
  printFailure(error);
  process.exitCode = 1;
} finally {
  await cleanup();
}
