#!/usr/bin/env node
// 一次性：把存量库里 visibility 两个菜单标题改成新文案（menu_seed 只补插不更新）
import { decodeTextCaptchaCode } from './api-smoke-lib.mjs';

const BASE = process.env.API_BASE_URL || 'http://localhost:8000/api/v1';

async function req(method, path, body, token) {
  const res = await fetch(BASE + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (res.status >= 300 || (data.code && data.code >= 300)) {
    throw new Error(`${method} ${path} -> ${res.status} ${data.message}`);
  }
  return data;
}

const cap = await req('GET', '/captcha');
const code = decodeTextCaptchaCode(cap.data.image);
const login = await req('POST', '/login', { username: 'admin', password: 'admin123', captcha_id: cap.data.key, captcha_code: code });
const token = login.data.access_token;

const updates = [
  { id: 95, title: '搜索排名' },
  { id: 96, title: 'AI 提及检测' },
];

for (const { id, title } of updates) {
  const cur = (await req('GET', `/menus/${id}`, null, token)).data;
  if (cur.title === title) {
    console.log(`menu ${id} already "${title}", skip`);
    continue;
  }
  await req('PUT', `/menus/${id}`, {
    name: cur.name,
    title,
    icon: cur.icon,
    path: cur.path,
    component: cur.component,
    parent_id: cur.parent_id,
    sort: cur.sort,
    status: cur.status,
    hidden: cur.hidden,
    permission: cur.permission || '',
  }, token);
  console.log(`menu ${id}: "${cur.title}" -> "${title}" ✔`);
}
console.log('DONE');
