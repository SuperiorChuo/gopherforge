#!/usr/bin/env node
// visibility 模块远端验证：登录 → SERP 采集(轮询) → GEO stub 跑题(轮询) → GEO llm 跑题(轮询)
// 用法: node visibility-verify.mjs [--skip-llm]
import { decodeTextCaptchaCode } from './api-smoke-lib.mjs';

const BASE = process.env.API_BASE_URL || 'http://localhost:8000/api/v1';
const USERNAME = process.env.SMOKE_USERNAME || 'admin';
const PASSWORD = process.env.SMOKE_PASSWORD || 'admin123';
const SKIP_LLM = process.argv.includes('--skip-llm');

let token = '';

async function req(method, path, body, expectFail = false) {
  const res = await fetch(BASE + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!expectFail && (res.status >= 300 || (data.code && data.code >= 300))) {
    throw new Error(`${method} ${path} -> HTTP ${res.status} code=${data.code} msg=${data.message}`);
  }
  return { status: res.status, ...data };
}

async function login() {
  const cap = await req('GET', '/captcha');
  const code = decodeTextCaptchaCode(cap.data.image);
  const res = await req('POST', '/login', { username: USERNAME, password: PASSWORD, captcha_id: cap.data.key, captcha_code: code });
  token = res.data.access_token || res.data.token;
  if (!token) throw new Error('no token in login response');
  console.log('✔ login ok');
}

async function poll(path, isDone, label, maxSec = 600) {
  const start = Date.now();
  for (;;) {
    const res = await req('GET', path);
    const d = res.data;
    if (isDone(d)) return d;
    if ((Date.now() - start) / 1000 > maxSec) throw new Error(`${label} poll timeout`);
    process.stdout.write(`\r  ${label} ... ${JSON.stringify(progressOf(d)).slice(0, 90)}   `);
    await new Promise((r) => setTimeout(r, 3000));
  }
}
function progressOf(d) {
  const t = d.task || d.run || d;
  return { status: t.status, done: t.done_count, total: t.total ?? t.prompt_count };
}

async function main() {
  await login();

  // 0. 确保有品牌/关键词/题库（空则种演示数据）
  // demo_mode 由后端给：生产环境（VISIBILITY_DEMO_MODE 关）不挂 seed-demo 路由、
  // 也不接受 mode=stub，本脚本对应步骤要主动跳过而不是撞 404/400。
  const ov = await req('GET', '/visibility/overview');
  const demoMode = ov.data.demo_mode !== false;
  if (!demoMode) console.log('ℹ 目标环境已关闭演示数据（demo_mode=false）：跳过 seed-demo 与 GEO stub 步骤');
  if (!ov.data.stats.brand_count || !ov.data.stats.keyword_count || !ov.data.stats.geo_prompt_count) {
    if (!demoMode) throw new Error('租户数据为空且目标环境禁用演示数据播种，请先在控制台录入品牌/关键词/检测问题');
    await req('POST', '/visibility/seed-demo', {});
    console.log('✔ seeded demo data');
  }
  // 给缺 target_url 的关键词补上（演示数据没填，serp 需要）
  const kws = await req('GET', '/visibility/keywords?page=1&page_size=50');
  for (const k of kws.data.list) {
    if (!k.target_url && k.status === 'tracking') {
      await req('PUT', `/visibility/keywords/${k.id}`, { target_url: 'https://github.com/go-admin-team/go-admin' });
    }
  }
  console.log(`✔ keywords ready: ${kws.data.total}`);

  // 1. SERP 采集
  const serp = await req('POST', '/visibility/serp/checks', {});
  console.log(`✔ serp task submitted id=${serp.data.task.id} reused=${serp.data.reused}`);
  const serpDone = await poll(`/visibility/serp/checks/${serp.data.task.id}`, (d) => d.task.status !== 'running', 'serp', 900);
  console.log(`\n✔ serp task ${serpDone.task.status}: total=${serpDone.task.total} ok=${serpDone.task.ok_count} fail=${serpDone.task.fail_count}`);

  // 排名结果 + 历史
  const kws2 = await req('GET', '/visibility/keywords?page=1&page_size=50');
  for (const k of kws2.data.list.slice(0, 5)) {
    console.log(`  [${k.keyword}] best=${k.rank} baidu=${k.baidu_rank} bing=${k.bing_rank} google=${k.google_rank}`);
  }
  const firstKw = kws2.data.list[0];
  if (firstKw) {
    const hist = await req('GET', `/visibility/keywords/${firstKw.id}/history?days=1`);
    const errs = hist.data.list.filter((s) => s.error);
    console.log(`✔ history snapshots=${hist.data.list.length}, failed engines: ${errs.map((e) => `${e.engine}(${e.error.slice(0, 40)})`).join('; ') || 'none'}`);
  }

  // 2. GEO stub 跑题（仅演示环境；生产下 mode=stub 会被后端拒绝）
  if (demoMode) {
    const stub = await req('POST', '/visibility/geo/runs', { mode: 'stub' });
    console.log(`✔ geo stub run id=${stub.data.run.id} reused=${stub.data.reused}`);
    const stubDone = await poll(`/visibility/geo/runs/${stub.data.run.id}`, (d) => d.run.status !== 'running', 'geo-stub', 120);
    console.log(`\n✔ geo stub ${stubDone.run.status}: mention ${stubDone.run.mention_count}/${stubDone.run.prompt_count}, results=${stubDone.results.length}, analyze_by=${stubDone.results[0]?.analyze_by}`);
  } else {
    // 闸门本身也要验证：关演示时 stub 必须被明确拒绝，而不是静默跑出假数据
    const denied = await req('POST', '/visibility/geo/runs', { mode: 'stub' }, true);
    if (denied.status < 300 && !(denied.code >= 300)) throw new Error('demo_mode=false 时 mode=stub 仍被接受');
    console.log('✔ geo stub 已按预期被拒绝（生产不产出模拟数据）');
  }

  // 3. GEO llm 跑题（真实中转站）
  if (!SKIP_LLM) {
    const llm = await req('POST', '/visibility/geo/runs', { mode: 'llm' });
    console.log(`✔ geo llm run id=${llm.data.run.id} reused=${llm.data.reused}`);
    // 立刻重复提交验证去重
    const dup = await req('POST', '/visibility/geo/runs', { mode: 'llm' });
    console.log(`✔ dedup check: reused=${dup.data.reused} (expect true)`);
    const llmDone = await poll(`/visibility/geo/runs/${llm.data.run.id}`, (d) => d.run.status !== 'running', 'geo-llm', 900);
    console.log(`\n✔ geo llm ${llmDone.run.status}: mention ${llmDone.run.mention_count}/${llmDone.run.prompt_count}`);
    for (const r of llmDone.results) {
      console.log(`  Q: ${r.question.slice(0, 30)}… engine=${r.engine} analyze=${r.analyze_by} sentiment=${r.sentiment || '-'} pos=${r.list_position} rec=${r.recommended}`);
    }
  }

  console.log('\nALL CHECKS DONE');
}

main().catch((e) => { console.error('\n✘ FAILED:', e.message); process.exit(1); });
