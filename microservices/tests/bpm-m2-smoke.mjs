#!/usr/bin/env node
// BPM M2 冒烟：对准远程环境（网关 18100）验证审批流 M2 全链路。
// 用法：API_BASE_URL=http://localhost:8000/api/v1 node tests/bpm-m2-smoke.mjs
// 覆盖：建流程(审批×2+抄送)→发起→退回上一节点→退回发起人→重提→委派办结(M3+)→
//       加签(M3+)→转办给冒烟用户→跨用户审批→终态 approved→时间线回放→
//       抄送已读闭环→CRM 回款审批(拒绝路径,不污染统计)。
// 专用资源 smoke- 前缀；流程定义收尾 suspend、冒烟用户删除；回款走拒绝路径保持统计归零。

import { buildConfig, decodeTextCaptchaCode, jsonObject } from './api-smoke-lib.mjs';

const config = buildConfig();
const state = {
  token: '', lastStep: 'init',
  created: { defId: null, payDefId: null, smokeUserId: null, instanceId: null },
};

function step(name) { state.lastStep = name; console.log(`[bpm-m2] ${name}`); }
function fail(msg, extra = '') {
  console.error(`\n[bpm-m2] FAILED at: ${state.lastStep}\nReason: ${msg}${extra ? `\n${extra}` : ''}`);
  process.exitCode = 1;
}

async function req(method, path, body, expected = 200, token = state.token) {
  const init = {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  };
  const resp = await fetch(`${config.apiBaseUrl}${path}`, init);
  const text = await resp.text();
  let data = null;
  try { data = JSON.parse(text); } catch { /* 允许非 JSON */ }
  if (resp.status !== expected) {
    throw new Error(`${method} ${path} -> ${resp.status}（期望 ${expected}）: ${text.slice(0, 300)}`);
  }
  return { status: resp.status, data };
}

async function loginAs(username, password) {
  const cap = await req('GET', '/captcha', null, 200, '');
  const captcha = cap.data?.data;
  const code = decodeTextCaptchaCode(captcha?.image ?? '');
  const res = await req('POST', '/login', JSON.parse(jsonObject({
    username, password, captcha_id: captcha?.key, captcha_code: code,
  })), 200, '');
  const token = res.data?.data?.access_token || '';
  if (!token) throw new Error(`登录 ${username} 未取得 access_token`);
  return token;
}

async function findTodo(token, pred, label) {
  for (let i = 0; i < 10; i++) {
    const res = await req('GET', '/bpm/tasks/todo?page=1&page_size=50', null, 200, token);
    const list = res.data?.data?.list ?? [];
    const hit = list.find(pred);
    if (hit) return hit;
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`待办中未找到：${label}`);
}

async function taskActions(token, taskId) {
  const res = await req('GET', `/bpm/tasks/${taskId}`, null, 200, token);
  return { actions: res.data?.data?.actions ?? [], detail: res.data?.data };
}

async function main() {
  step('管理员登录');
  state.token = await loginAs(config.username, config.password);

  step('取管理员用户 id');
  const users = await req('GET', `/users?keyword=${encodeURIComponent(config.username)}&page=1&page_size=10`);
  const admin = (users.data?.data?.list ?? []).find((u) => u.username === config.username);
  if (!admin) throw new Error('用户列表未找到管理员');
  const adminId = admin.id;

  step('建冒烟用户（转办目标）');
  const su = await req('POST', '/users', {
    username: 'smoke-m2-user', password: 'Smoke@2026Ab', nickname: '冒烟转办员', status: 1,
  }).catch(async (e) => {
    // 已存在则查出来复用
    if (!String(e).includes('exists') && !String(e).includes('已存在')) throw e;
    return null;
  });
  let smokeUserId = su?.data?.data?.id;
  if (!smokeUserId) {
    const q = await req('GET', '/users?keyword=smoke-m2-user&page=1&page_size=10');
    smokeUserId = (q.data?.data?.list ?? []).find((u) => u.username === 'smoke-m2-user')?.id;
  }
  if (!smokeUserId) throw new Error('冒烟用户不可得');
  state.created.smokeUserId = smokeUserId;

  step('建流程定义（审批A→抄送→审批B[允许退回上一节点]）');
  const tree = {
    version: 1,
    start: {
      id: 'n-start', name: '发起', type: 'start', formFields: ['amount_cents'],
      next: {
        id: 'n-a1', name: '一级审批', type: 'approval', multiMode: 'OR',
        assignee: { type: 'users', userIds: [adminId] },
        next: {
          id: 'n-cc', name: '抄送管理员', type: 'cc',
          targets: { type: 'users', userIds: [adminId] },
          next: {
            id: 'n-a2', name: '二级审批', type: 'approval', multiMode: 'OR',
            allowBackPrev: true,
            assignee: { type: 'users', userIds: [adminId] },
          },
        },
      },
    },
  };
  // 用户侧发起要求定义带 form_schema（表单构建器 M1 安全收口）
  const formSchema = {
    version: 1,
    fields: [{ key: 'amount_cents', label: '金额', type: 'amount', required: true }],
  };
  // 幂等：key 残留（上次冒烟 suspend 不删 key）时改走"发新版本"路径
  const def = await req('POST', '/bpm/definitions', {
    key: 'smoke_m2_flow', name: 'smoke-M2全链路', biz_type: 'smoke_m2',
    node_tree: tree, form_schema: formSchema,
  }).catch(async (e) => {
    if (!String(e).includes('已存在')) throw e;
    const q = await req('GET', '/bpm/definitions?keyword=smoke_m2_flow&page=1&page_size=50');
    const old = (q.data?.data?.list ?? []).find((d) => d.key === 'smoke_m2_flow');
    if (!old) throw new Error('smoke_m2_flow 定义存在但列表查不到');
    const nv = await req('POST', `/bpm/definitions/${old.id}/new-version`, null);
    const nvId = nv.data?.data?.id;
    await req('PUT', `/bpm/definitions/${nvId}`, { node_tree: tree, form_schema: formSchema });
    return { data: { data: { id: nvId } } };
  });
  state.created.defId = def.data?.data?.id;
  await req('POST', `/bpm/definitions/${state.created.defId}/publish`, null);

  step('发起实例');
  const inst = await req('POST', '/bpm/instances', {
    definition_key: 'smoke_m2_flow', title: 'smoke-M2 实例',
    biz_type: 'smoke_m2', biz_id: String(Date.now()),
    form_snapshot: { amount_cents: 5000 },
  });
  const instId = inst.data?.data?.instance_id;
  if (!instId) throw new Error('发起未返回 instance_id');
  state.created.instanceId = instId;

  step('一级审批：动作应含 transfer、不含 return_prev');
  let t1 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a1', '一级审批任务');
  let { actions: a1 } = await taskActions(state.token, t1.id);
  if (!a1.includes('transfer') || a1.includes('return_prev')) throw new Error(`一级动作异常: ${a1}`);
  await req('POST', `/bpm/tasks/${t1.id}/approve`, { comment: 'smoke 一级通过' });

  step('抄送箱：未读→已读闭环');
  const cc = await req('GET', '/bpm/cc/my?unread_only=true&page=1&page_size=50');
  const ccRec = (cc.data?.data?.list ?? []).find((r) => r.instance_id === instId);
  if (!ccRec) throw new Error('抄送记录未出现');
  await req('POST', `/bpm/cc/${ccRec.id}/read`, null);
  await req('POST', `/bpm/cc/${ccRec.id}/read`, null); // 幂等
  const cc2 = await req('GET', '/bpm/cc/my?page=1&page_size=50');
  const ccAfter = (cc2.data?.data?.list ?? []).find((r) => r.id === ccRec.id);
  if (!ccAfter?.read_at) throw new Error('标记已读未生效');

  step('二级审批：退回上一节点');
  let t2 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a2', '二级审批任务');
  const { actions: a2 } = await taskActions(state.token, t2.id);
  if (!a2.includes('return_prev')) throw new Error(`二级动作缺 return_prev: ${a2}`);
  await req('POST', `/bpm/tasks/${t2.id}/return`, { to: 'prev', comment: 'smoke 退回上一节点' });

  step('一级审批 round2 重现 → 通过');
  t1 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a1' && t.round === 2, '一级 round2');
  await req('POST', `/bpm/tasks/${t1.id}/approve`, { comment: 'smoke 一级二轮通过' });

  step('二级审批 round2：退回发起人');
  t2 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a2' && t.round === 2, '二级 round2');
  await req('POST', `/bpm/tasks/${t2.id}/return`, { to: 'start', comment: 'smoke 退回发起人' });

  step('发起人重提任务：动作应含 resubmit → 改快照重提');
  const rt = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-start', '重提任务');
  const { actions: ra } = await taskActions(state.token, rt.id);
  if (!ra.includes('resubmit')) throw new Error(`重提任务动作异常: ${ra}`);
  await req('POST', `/bpm/instances/${instId}/resubmit`, { form_snapshot: { amount_cents: 6000 } });

  step('冒烟用户登录（委派/加签/转办目标）');
  const suToken = await loginAs('smoke-m2-user', 'Smoke@2026Ab');

  step('一级 round3：委派给冒烟用户 → 受托人办结 → 任务回到管理员');
  t1 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a1' && t.round === 3, '一级 round3');
  await req('POST', `/bpm/tasks/${t1.id}/delegate`, { target_user_id: smokeUserId, comment: 'smoke 委派核实' });
  const dgone = await req('GET', '/bpm/tasks/todo?page=1&page_size=50');
  if ((dgone.data?.data?.list ?? []).some((t) => t.id === t1.id)) throw new Error('委派后原人待办仍在');
  const dt = await findTodo(suToken, (t) => t.id === t1.id, '委派中的任务');
  if (!dt.delegated_by) throw new Error('受托人待办行缺 delegated_by 标记');
  const { actions: da } = await taskActions(suToken, t1.id);
  if (da.length !== 1 || da[0] !== 'delegate_resolve') throw new Error(`委派中动作应仅 delegate_resolve: ${da}`);
  await req('POST', `/bpm/tasks/${t1.id}/approve`, { comment: '不应成功' }, 400, suToken);
  await req('POST', `/bpm/tasks/${t1.id}/delegate/resolve`, { comment: '' }, 400, suToken); // 意见必填
  await req('POST', `/bpm/tasks/${t1.id}/delegate/resolve`, { comment: 'smoke 已核实办结' }, 200, suToken);
  await findTodo(state.token, (t) => t.id === t1.id, '办结后回到管理员的任务');

  step('一级 round3：加签冒烟用户（同节点并列参与或签）');
  await req('POST', `/bpm/tasks/${t1.id}/add-sign`, { user_ids: [smokeUserId], comment: 'smoke 加签' });
  const ast = await findTodo(
    suToken,
    (t) => t.instance_id === instId && t.node_id === 'n-a1' && t.round === 3 && t.id !== t1.id,
    '加签任务',
  );
  if (!ast.add_sign_by) throw new Error('加签待办行缺 add_sign_by 标记');

  step('一级审批 round3 → 转办给冒烟用户');
  await req('POST', `/bpm/tasks/${t1.id}/transfer`, { target_user_id: smokeUserId, comment: 'smoke 转办' });
  const gone = await req('GET', '/bpm/tasks/todo?page=1&page_size=50');
  if ((gone.data?.data?.list ?? []).some((t) => t.id === t1.id)) throw new Error('转办后原人待办仍在');

  step('冒烟用户接手审批（或签通过后加签任务应被跳过）');
  const st = await findTodo(suToken, (t) => t.id === t1.id, '转办后的任务');
  await req('POST', `/bpm/tasks/${st.id}/approve`, { comment: 'smoke 转办人通过' }, 200, suToken);
  const suAfter = await req('GET', '/bpm/tasks/todo?page=1&page_size=50', null, 200, suToken);
  if ((suAfter.data?.data?.list ?? []).some((t) => t.id === ast.id)) {
    throw new Error('或签通过后加签任务待办应消失（skipped）');
  }

  step('二级审批 round3 → 通过 → 实例 approved');
  t2 = await findTodo(state.token, (t) => t.instance_id === instId && t.node_id === 'n-a2' && t.round === 3, '二级 round3');
  await req('POST', `/bpm/tasks/${t2.id}/approve`, { comment: 'smoke 终审通过' });
  const fin = await req('GET', `/bpm/instances/${instId}`);
  if (fin.data?.data?.status !== 'approved' && fin.data?.data?.instance?.status !== 'approved') {
    throw new Error(`实例未 approved: ${JSON.stringify(fin.data?.data).slice(0, 200)}`);
  }

  step('时间线回放：应含 M2 + 加签/委派动作');
  const tl = await req('GET', `/bpm/instances/${instId}/timeline`);
  const acts = new Set((tl.data?.data?.list ?? tl.data?.data ?? []).map((l) => l.action));
  for (const a of ['transfer', 'return_prev', 'return_start', 'resubmit', 'add_sign', 'delegate', 'delegate_resolve']) {
    if (!acts.has(a)) throw new Error(`时间线缺动作 ${a}（有：${[...acts].join(',')}）`);
  }

  step('回款审批：建支付流程定义（key=crm_payment_approval）');
  const existing = await req('GET', '/bpm/definitions/keys/crm_payment_approval/active', null, 200)
    .catch(() => null);
  if (!existing?.data?.data?.id) {
    const pd = await req('POST', '/bpm/definitions', {
      key: 'crm_payment_approval', name: '回款审批', biz_type: 'crm_payment',
      node_tree: {
        version: 1,
        start: {
          id: 'p-start', name: '发起', type: 'start', formFields: ['amount_cents'],
          next: {
            id: 'p-a1', name: '财务确认', type: 'approval', multiMode: 'OR',
            assignee: { type: 'users', userIds: [adminId] },
          },
        },
      },
    });
    state.created.payDefId = pd.data?.data?.id;
    await req('POST', `/bpm/definitions/${state.created.payDefId}/publish`, null);
  }

  step('回款审批：登记回款（拒绝路径，不污染统计）');
  const contracts = await req('GET', '/crm/contracts?status=active&page=1&page_size=1');
  const contract = (contracts.data?.data?.list ?? [])[0];
  if (!contract) {
    console.log('[bpm-m2] 无 active 合同，回款段跳过（引擎链路已由前序覆盖）');
  } else {
    const before = await req('GET', `/crm/contracts/${contract.id}`);
    const paidBefore = before.data?.data?.paid_cents ?? 0;
    const pay = await req('POST', `/crm/contracts/${contract.id}/payments`, {
      amount_cents: 1, paid_at: new Date().toISOString(), method: 'other', note: 'smoke-m2 回款审批冒烟（将拒绝）',
    });
    const payData = pay.data?.data ?? {};
    if (payData.status !== 'pending_approval') throw new Error(`回款未进入审批态: ${JSON.stringify(payData).slice(0, 200)}`);
    const mid = await req('GET', `/crm/contracts/${contract.id}`);
    if ((mid.data?.data?.paid_cents ?? 0) !== paidBefore) throw new Error('审批中回款不应计入统计');
    const pt = await findTodo(state.token, (t) => t.node_id === 'p-a1', '回款审批任务');
    await req('POST', `/bpm/tasks/${pt.id}/reject`, { comment: 'smoke 拒绝，勿计入' });
    const after = await req('GET', `/crm/contracts/${contract.id}`);
    if ((after.data?.data?.paid_cents ?? 0) !== paidBefore) throw new Error('拒绝后回款不应计入统计');
    console.log('[bpm-m2] 回款审批链路 OK（pending_approval → rejected，统计未变）');
  }

  console.log('\n[bpm-m2] 全部通过 ✅');
}

async function cleanup() {
  step('清理');
  try {
    if (state.created.defId) await req('POST', `/bpm/definitions/${state.created.defId}/suspend`, null);
  } catch (e) { console.warn('挂起冒烟定义失败（不影响结果）:', String(e).slice(0, 120)); }
  try {
    if (state.created.smokeUserId) await req('DELETE', `/users/${state.created.smokeUserId}`, null);
  } catch (e) { console.warn('删除冒烟用户失败（不影响结果）:', String(e).slice(0, 120)); }
  // crm_payment_approval 定义保留：即是生产所需的回款审批流程
}

main().then(cleanup).catch(async (e) => { fail(e.message || String(e)); await cleanup().catch(() => {}); });
