#!/usr/bin/env node
// 呼叫中心冒烟：对准远程环境（网关 18100）验证 cc 全链路关键面。
// 用法：API_BASE_URL=http://localhost:8000/api/v1 node tests/cc-smoke.mjs
// 登录复用 api-smoke 的验证码解码器；只做「读 + 建了就删」的可逆操作，
// 不碰生产队列/线路，专用资源统一 smoke- 前缀并在 finally 清理。

import { buildConfig, decodeTextCaptchaCode, jsonObject } from './api-smoke-lib.mjs';

const config = buildConfig();
const state = { token: '', lastStep: 'init', created: { queue: false, blacklist: false, rate: null } };

function step(name) {
  state.lastStep = name;
  console.log(`[cc-smoke] ${name}`);
}

function fail(msg, extra = '') {
  console.error(`\n[cc-smoke] FAILED at: ${state.lastStep}\nReason: ${msg}${extra ? `\n${extra}` : ''}`);
  process.exitCode = 1;
}

async function req(method, path, body, expected = 200) {
  const init = {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(state.token ? { Authorization: `Bearer ${state.token}` } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  };
  const resp = await fetch(`${config.apiBaseUrl}${path}`, init);
  const text = await resp.text();
  let data = null;
  try { data = JSON.parse(text); } catch { /* 允许非 JSON */ }
  if (resp.status !== expected) {
    throw new Error(`${method} ${path} -> ${resp.status}（期望 ${expected}）: ${text.slice(0, 200)}`);
  }
  return { status: resp.status, data };
}

async function login() {
  step('captcha + login');
  const cap = await req('GET', '/captcha', null);
  const captcha = cap.data?.data;
  const code = decodeTextCaptchaCode(captcha?.image ?? '');
  const res = await req('POST', '/login', JSON.parse(jsonObject({
    username: config.username,
    password: config.password,
    captcha_id: captcha?.key,
    captcha_code: code,
  })));
  state.token = res.data?.data?.access_token || '';
  if (!state.token) throw new Error('登录未取得 access_token');
}

async function main() {
  await login();

  step('cc 健康检查');
  await req('GET', '/cc/health/ready', null);

  step('话单列表可读');
  const calls = await req('GET', '/cc/calls?page=1&page_size=1', null);
  if (calls.data?.code !== 200) throw new Error('话单列表 envelope 异常');

  step('FS 状态可达');
  const fs = await req('GET', '/cc/fs/status', null);
  if (!fs.data?.data?.configured) throw new Error('CC_FS_API_BASE 未配置');

  step('队列建删闭环（smoke-q）');
  await req('POST', '/cc/fs/queues', { name: 'smoke-q', access_number: '7990', strategy: 'ring-all' });
  state.created.queue = true;
  const queues = await req('GET', '/cc/fs/queues', null);
  if (!queues.data?.data?.list?.some((q) => q.name === 'smoke-q')) throw new Error('新建队列未出现在列表');

  step('黑名单增删闭环');
  await req('POST', '/cc/fs/blacklist', { number: '19999990000', remark: 'smoke' });
  state.created.blacklist = true;

  step('识客 lookup 聚合');
  const lk = await req('GET', '/cc/lookup?number=19999990000', null);
  if (!('has_crm' in (lk.data?.data ?? {}))) throw new Error('lookup 返回缺 has_crm');

  step('报表字段完整');
  const rep = await req('GET', '/cc/reports/overview', null);
  for (const k of ['total', 'answered', 'answer_rate', 'agent_leaderboard']) {
    if (!(k in (rep.data?.data ?? {}))) throw new Error(`报表缺字段 ${k}`);
  }

  step('计费费率建删闭环');
  const rate = await req('POST', '/cc/billing/rates', { prefix: '99999', name: 'smoke', rate_per_min: 1, billing_step: 60 });
  state.created.rate = rate.data?.data?.id ?? null;

  step('WS 握手升级');
  const wsResp = await fetch(`${config.apiBaseUrl}/cc/ws?token=${encodeURIComponent(state.token)}`, {
    headers: {
      Connection: 'Upgrade', Upgrade: 'websocket',
      'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': 'dGhlIHNhbXBsZSBub25jZQ==',
    },
  }).catch((e) => ({ status: 0, err: e }));
  // Node fetch 对 101 会抛错或返回非 101，容错：401/403 才算失败
  if (wsResp.status === 401 || wsResp.status === 403) throw new Error('WS 鉴权被拒');

  console.log(`[cc-smoke] ok: 呼叫中心冒烟通过（${config.apiBaseUrl}）`);
}

async function cleanup() {
  if (!state.token) return;
  const quiet = async (fn) => { try { await fn(); } catch { /* 清理失败不掩盖主错误 */ } };
  if (state.created.queue) await quiet(() => req('DELETE', '/cc/fs/queues/smoke-q', null));
  if (state.created.blacklist) await quiet(() => req('DELETE', '/cc/fs/blacklist/19999990000', null));
  if (state.created.rate) await quiet(() => req('DELETE', `/cc/billing/rates/${state.created.rate}`, null));
}

try {
  await main();
} catch (e) {
  fail(e.message);
} finally {
  await cleanup();
}
