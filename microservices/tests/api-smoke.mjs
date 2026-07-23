#!/usr/bin/env node

import { createHash, randomBytes, createPublicKey, verify as cryptoVerify } from 'node:crypto';

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
  oauth2ClientDbId: '',
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
  console.error('1. Start dependencies and backend: docker compose up -d go-admin-kit-postgres go-admin-kit-redis, then start backend.');
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

// OAuth2 protocol endpoints (token/introspect/revoke) speak
// application/x-www-form-urlencoded and return bare RFC JSON.
async function requestForm(path, expected, fields, token = '') {
  const headers = {
    Accept: 'application/json',
    'Content-Type': 'application/x-www-form-urlencoded',
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
      body: new URLSearchParams(fields).toString(),
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

function pkcePair() {
  const verifier = randomBytes(32).toString('base64url');
  const challenge = createHash('sha256').update(verifier).digest('base64url');
  return { verifier, challenge };
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

  if (state.oauth2ClientDbId && state.accessToken) {
    await cleanupRequest('DELETE', `/oauth2/clients/${state.oauth2ClientDbId}`);
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

  // ---------- OAuth2 授权服务端全流程 ----------
  const oauth2Redirect = 'https://smoke.example.com/callback';
  const oauth2Nonce = `smoke-nonce-${config.safeRunId}`;
  const { verifier: pkceVerifier, challenge: pkceChallenge } = pkcePair();

  step('oauth2 create client');
  response = await request('POST', '/oauth2/clients', 200, jsonObject({
    name: `smoke-oauth2-${config.safeRunId}`,
    client_type: 1,
    redirect_uris: [oauth2Redirect],
    scopes: ['openid', 'profile', 'email'],
    grant_types: ['authorization_code', 'refresh_token', 'client_credentials'],
  }), state.accessToken);
  const oauth2ClientId = getJsonPath(response.data, 'data.client.client_id');
  const oauth2ClientSecret = getJsonPath(response.data, 'data.client_secret');
  state.oauth2ClientDbId = getJsonPath(response.data, 'data.client.id');
  assertResponse(
    response.data?.code === 200 && typeof oauth2ClientId === 'string' && typeof oauth2ClientSecret === 'string' && oauth2ClientSecret.length > 0,
    'oauth2 client create did not return client_id + one-time secret',
  );

  const authorizeQuery =
    `?response_type=code&client_id=${encodeURIComponent(oauth2ClientId)}` +
    `&redirect_uri=${encodeURIComponent(oauth2Redirect)}&scope=${encodeURIComponent('openid profile email')}` +
    `&state=smoke-state&nonce=${encodeURIComponent(oauth2Nonce)}&code_challenge=${pkceChallenge}&code_challenge_method=S256`;

  step('oauth2 authorize view');
  response = await request('GET', `/oauth2/authorize${authorizeQuery}`, 200, '', state.accessToken);
  assertResponse(
    response.data?.code === 200 && response.data?.data?.client_id === oauth2ClientId && Array.isArray(response.data?.data?.scopes),
    'oauth2 authorize view did not return the consent payload',
  );

  step('oauth2 approve -> code');
  response = await request('POST', '/oauth2/authorize', 200, jsonObject({
    client_id: oauth2ClientId,
    redirect_uri: oauth2Redirect,
    response_type: 'code',
    scope: 'openid profile email',
    state: 'smoke-state',
    nonce: oauth2Nonce,
    code_challenge: pkceChallenge,
    code_challenge_method: 'S256',
    approved: true,
  }), state.accessToken);
  const approveRedirect = getJsonPath(response.data, 'data.redirect_url');
  assertResponse(response.data?.code === 200 && typeof approveRedirect === 'string', 'oauth2 approve did not return a redirect_url');
  const approveUrl = new URL(approveRedirect);
  const authCode = approveUrl.searchParams.get('code');
  assertResponse(
    authCode && approveUrl.searchParams.get('state') === 'smoke-state',
    'oauth2 redirect is missing code or state',
  );

  step('oauth2 token (authorization_code + PKCE)');
  let tokenRes = await requestForm('/oauth2/token', 200, {
    grant_type: 'authorization_code',
    code: authCode,
    redirect_uri: oauth2Redirect,
    code_verifier: pkceVerifier,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  const oauthAccess = tokenRes.data?.access_token;
  const oauthRefresh = tokenRes.data?.refresh_token;
  const oauthIDToken = tokenRes.data?.id_token;
  assertResponse(
    tokenRes.data?.token_type === 'Bearer' && typeof oauthAccess === 'string' && typeof oauthRefresh === 'string',
    'oauth2 token exchange did not return bearer access + refresh tokens',
  );
  assertResponse(typeof oauthIDToken === 'string' && oauthIDToken.split('.').length === 3, 'openid scope did not yield a JWT id_token');

  step('oidc discovery document');
  response = await request('GET', '/oauth2/.well-known/openid-configuration', 200);
  const disco = response.data;
  assertResponse(
    typeof disco?.issuer === 'string' &&
      disco.issuer.endsWith('/api/v1/oauth2') &&
      typeof disco.jwks_uri === 'string' &&
      Array.isArray(disco.id_token_signing_alg_values_supported) &&
      disco.id_token_signing_alg_values_supported.includes('RS256'),
    'oidc discovery document is missing required fields',
  );

  step('oidc jwks + id_token signature verify');
  const jwksRes = await request('GET', '/oauth2/jwks', 200);
  const jwk = jwksRes.data?.keys?.[0];
  assertResponse(jwk?.kty === 'RSA' && typeof jwk.n === 'string' && typeof jwk.kid === 'string', 'jwks did not return an RSA key');
  // 用 JWKS 公钥验证 id_token 的 RS256 签名（node 内置 crypto 直接吃 JWK）
  const [idHeaderB64, idPayloadB64, idSigB64] = oauthIDToken.split('.');
  const idPubKey = createPublicKey({ key: { kty: jwk.kty, n: jwk.n, e: jwk.e }, format: 'jwk' });
  const idSigOk = cryptoVerify(
    'RSA-SHA256',
    Buffer.from(`${idHeaderB64}.${idPayloadB64}`),
    idPubKey,
    Buffer.from(idSigB64, 'base64url'),
  );
  assertResponse(idSigOk, 'id_token RS256 signature did not verify against JWKS');
  const idHeader = JSON.parse(Buffer.from(idHeaderB64, 'base64url').toString('utf8'));
  const idClaims = JSON.parse(Buffer.from(idPayloadB64, 'base64url').toString('utf8'));
  assertResponse(idHeader.alg === 'RS256' && idHeader.kid === jwk.kid, 'id_token header alg/kid mismatch with JWKS');
  assertResponse(
    idClaims.iss === disco.issuer &&
      idClaims.aud === oauth2ClientId &&
      idClaims.nonce === oauth2Nonce &&
      typeof idClaims.sub === 'string' &&
      idClaims.email !== undefined,
    'id_token claims (iss/aud/nonce/sub/email) are not as expected',
  );

  step('oauth2 userinfo');
  response = await request('GET', '/oauth2/userinfo', 200, '', oauthAccess);
  assertResponse(
    response.data?.sub !== undefined && response.data?.username === config.username && response.data?.email !== undefined,
    'oauth2 userinfo did not return scoped claims',
  );

  step('oauth2 introspect active');
  response = await requestForm('/oauth2/introspect', 200, {
    token: oauthAccess,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  assertResponse(response.data?.active === true && response.data?.client_id === oauth2ClientId, 'introspect did not report the token active');

  step('oauth2 refresh rotation');
  tokenRes = await requestForm('/oauth2/token', 200, {
    grant_type: 'refresh_token',
    refresh_token: oauthRefresh,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  const rotatedAccess = tokenRes.data?.access_token;
  assertResponse(typeof rotatedAccess === 'string' && rotatedAccess !== oauthAccess, 'oauth2 refresh did not mint a new access token');

  step('oauth2 old refresh token rejected');
  tokenRes = await requestForm('/oauth2/token', 400, {
    grant_type: 'refresh_token',
    refresh_token: oauthRefresh,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  assertResponse(tokenRes.data?.error === 'invalid_grant', 'reused refresh token was not rejected with invalid_grant');

  step('oauth2 client_credentials grant');
  tokenRes = await requestForm('/oauth2/token', 200, {
    grant_type: 'client_credentials',
    scope: 'profile',
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  const ccAccess = tokenRes.data?.access_token;
  assertResponse(typeof ccAccess === 'string' && tokenRes.data?.refresh_token === undefined, 'client_credentials should mint an access token without a refresh token');

  step('oauth2 client_credentials has no user (userinfo 403)');
  response = await request('GET', '/oauth2/userinfo', 403, '', ccAccess);
  assertResponse(response.data?.error === 'insufficient_scope', 'client_credentials token should have no user for userinfo');

  step('oauth2 revoke access token');
  await requestForm('/oauth2/revoke', 200, {
    token: rotatedAccess,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  response = await requestForm('/oauth2/introspect', 200, {
    token: rotatedAccess,
    client_id: oauth2ClientId,
    client_secret: oauth2ClientSecret,
  });
  assertResponse(response.data?.active === false, 'revoked access token still reports active');

  step('oauth2 delete client');
  response = await request('DELETE', `/oauth2/clients/${state.oauth2ClientDbId}`, 200, '', state.accessToken);
  assertResponse(response.data?.code === 200, 'oauth2 client delete did not succeed');
  state.oauth2ClientDbId = '';

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
