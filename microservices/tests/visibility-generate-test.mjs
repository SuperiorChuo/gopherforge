#!/usr/bin/env node
// 实测 GEO「AI 生成问题」：generate 走中转站返回候选 → batch 落库 → 清理测试数据
import { decodeTextCaptchaCode } from './api-smoke-lib.mjs';

const BASE = process.env.API_BASE_URL || 'http://localhost:8000/api/v1';

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
if (!token) { console.error('登录失败', login); process.exit(1); }
console.log('✅ 登录成功');

// 1) generate：走中转站
console.log('⏳ 调用 generate（LLM，可能要十几秒）...');
const gen = await req('POST', '/visibility/geo/prompts/generate', { hint: '面向中小企业技术团队的开源后台管理系统', count: 5 }, token);
if (gen.code !== 200) { console.error('❌ generate 失败:', gen); process.exit(1); }
const items = gen.data.items || [];
console.log(`✅ generate 返回 ${items.length} 个候选：`);
for (const it of items) console.log(`   [${it.category}] ${it.question}`);
if (items.length === 0) process.exit(1);

// 2) batch：落库前 2 个
const before = await req('GET', '/visibility/geo/prompts?page=1&page_size=100', null, token);
const batch = await req('POST', '/visibility/geo/prompts/batch', { items: items.slice(0, 2) }, token);
if (batch.code !== 200 || batch.data.created !== 2) { console.error('❌ batch 失败:', batch); process.exit(1); }
console.log(`✅ batch 落库 created=${batch.data.created}`);

// 3) 校验并清理测试数据（按问题文本匹配刚插入的两条）
const after = await req('GET', '/visibility/geo/prompts?page=1&page_size=100', null, token);
const beforeIds = new Set((before.data.list || []).map((p) => p.id));
const added = (after.data.list || []).filter((p) => !beforeIds.has(p.id));
console.log(`✅ 题库新增 ${added.length} 条（${before.data.total} → ${after.data.total}）`);
for (const p of added) {
  await req('DELETE', `/visibility/geo/prompts/${p.id}`, null, token);
}
console.log(`🧹 已清理 ${added.length} 条测试数据，题库恢复原状`);
console.log('🎉 AI 生成问题端到端验证通过');
