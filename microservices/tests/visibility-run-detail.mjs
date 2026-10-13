#!/usr/bin/env node
// 查看指定 GEO run 的失败详情（answer 里嵌了错误信息）
import { decodeTextCaptchaCode } from './api-smoke-lib.mjs';

const BASE = process.env.API_BASE_URL || 'http://localhost:8000/api/v1';
const runId = process.argv[2] || '5';

async function req(method, path, body, token) {
  const res = await fetch(BASE + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  return res.json();
}

const cap = await req('GET', '/captcha');
const code = decodeTextCaptchaCode(cap.data.image);
const login = await req('POST', '/login', { username: 'admin', password: 'admin123', captcha_id: cap.data.key, captcha_code: code });
const token = login.data.access_token;

const run = await req('GET', `/visibility/geo/runs/${runId}`, null, token);
for (const r of run.data.results) {
  console.log(`engine=${r.engine}`);
  console.log(r.answer.slice(0, 400));
  console.log('---');
}
