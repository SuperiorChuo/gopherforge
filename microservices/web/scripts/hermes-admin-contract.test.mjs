import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import {
  applyFulfilledValue,
  beginHermesRequest,
  clusterFormValues,
  clusterPayload,
  fetchAllHermesPages,
  HERMES_INSTANCE_STATUS_LABELS,
  instanceFormValues,
  instancePayload,
  invalidateHermesRequests,
  isPolicyToolsetValid,
  policyFormValues,
  policyPayload,
  policyToolsetCatalog,
  runLatestHermesRequest,
} from '../src/pages/hermes/admin-contract.ts'

const page = async (name) => readFile(new URL(`../src/pages/hermes/${name}/index.tsx`, import.meta.url), 'utf8')

function deferred() {
  let resolve
  let reject
  const promise = new Promise((onResolve, onReject) => {
    resolve = onResolve
    reject = onReject
  })
  return { promise, resolve, reject }
}

test('实例只改名称时保留 cluster_id，也支持显式解除归组', () => {
  const record = {
    id: 7,
    tenant_id: 1,
    name: '研发助手',
    role: 'research',
    owner_user_id: 2,
    device_id: 'device-1',
    policy_id: 3,
    cluster_id: 42,
    enabled: true,
    status: 'running',
    created_at: '',
    updated_at: '',
  }
  const values = instanceFormValues(record)
  assert.equal(instancePayload({ ...values, name: '研发助手 A' }).cluster_id, 42)
  assert.equal(instancePayload({ ...values, cluster_id: undefined }).cluster_id, 0)
})

test('实例页展示集群归属并提供 cluster_id 选择器', async () => {
  const source = await page('instances')
  assert.match(source, /listHermesClusters/)
  assert.match(source, /name="cluster_id"/)
  assert.match(source, /instanceFormValues\(record\)/)
  assert.match(source, /instancePayload\(values\)/)
})

test('集群只改名称时保留 slug，也支持显式切回 Push', () => {
  const record = {
    id: 8,
    tenant_id: 1,
    name: '研发集群',
    slug: 'research-team',
    description: '',
    policy_id: 9,
    enabled: true,
    created_at: '',
    updated_at: '',
  }
  const values = clusterFormValues(record)
  assert.equal(clusterPayload({ ...values, name: '研发集群 A' }).slug, 'research-team')
  assert.equal(clusterPayload({ ...values, slug: '' }).slug, '')
})

test('集群页展示并编辑 Kanban slug', async () => {
  const source = await page('clusters')
  assert.match(source, /name="slug"/)
  assert.match(source, /clusterFormValues\(record\)/)
  assert.match(source, /clusterPayload\(values\)/)
})

test('company_cluster 策略编辑保留 profile 与 delegation 工具集', () => {
  const policy = {
    id: 11,
    tenant_id: 1,
    revision: 2,
    name: 'dispatcher',
    desired_state: 'active',
    profile: 'company_cluster',
    assistant_template_id: '',
    assistant_template_revision: 0,
    artifact_id: 0,
    model_id: 0,
    model_name: '',
    model_base_url: '',
    toolsets: ['clarify', 'delegation', 'skills'],
    privacy: { upload_conversations: false, upload_memory: false, upload_local_files: false },
    lease_seconds: 3600,
    enable_knowledge_rag: false,
    created_at: '',
  }
  const values = policyFormValues(policy, ['clarify', 'memory', 'no_mcp', 'todo'])
  const p1 = ['clarify', 'memory', 'no_mcp', 'todo']
  const cluster = [...p1, 'delegation', 'skills', 'mcp']
  const payload = policyPayload({ ...values, id: 999, tenant_id: 888 }, p1, cluster)
  assert.equal(payload.profile, 'company_cluster')
  assert.deepEqual(payload.toolsets, ['clarify', 'delegation', 'skills'])
  assert.deepEqual(payload.privacy, {
    upload_conversations: false,
    upload_memory: false,
    upload_local_files: false,
  })
  assert.equal('id' in payload, false)
  assert.equal('tenant_id' in payload, false)
})

test('策略工具集拒绝未知项、重复项、缺失项与同长度错误集合', () => {
  const p1 = ['clarify', 'memory', 'no_mcp', 'todo']
  const cluster = [...p1, 'delegation', 'skills', 'mcp']
  assert.equal(isPolicyToolsetValid('company', p1, p1, cluster), true)
  assert.equal(isPolicyToolsetValid('company', ['clarify', 'memory', 'todo', 'unknown'], p1, cluster), false)
  assert.equal(isPolicyToolsetValid('company', ['clarify', 'memory', 'todo', 'todo'], p1, cluster), false)
  assert.equal(isPolicyToolsetValid('company_cluster', ['clarify', 'skills'], p1, cluster), false)
  assert.equal(isPolicyToolsetValid('company_cluster', ['clarify', 'delegation', 'kanban'], p1, cluster), false)
  assert.throws(
    () => policyPayload({ profile: 'company', name: 'bad', desired_state: 'active', toolsets: ['clarify', 'memory', 'todo', 'unknown'], lease_seconds: 3600 }, p1, cluster),
    /invalid Hermes policy toolsets/,
  )
})

test('策略工具目录使用 P1/P2 去重并集，且不包含 Kanban 通道', () => {
  const catalog = policyToolsetCatalog(
    ['clarify', 'memory', 'no_mcp', 'todo'],
    ['clarify', 'delegation', 'skills', 'mcp'],
  )
  assert.deepEqual(catalog, ['clarify', 'memory', 'no_mcp', 'todo', 'delegation', 'skills', 'mcp'])
  assert.equal(catalog.includes('kanban'), false)
})

test('策略页可选 profile，并按表单值提交对应 toolsets', async () => {
  const source = await page('policies')
  assert.match(source, /name="profile"/)
  assert.match(source, /name="toolsets"/)
  assert.match(source, /form\.resetFields\(\)/)
  assert.match(source, /policyFormValues\(policy, p1Toolsets\)/)
  assert.match(source, /policyPayload\(values, p1Toolsets, p2ClusterToolsets\)/)
  assert.match(source, /p2ClusterToolsets/)
})

test('Kanban 只作为集群通道能力展示，不冒充策略工具', async () => {
  const source = await page('skills')
  assert.match(source, /policyToolsetCatalog\(p1Toolsets, p2ClusterToolsets\)/)
  assert.match(source, /CHANNEL_CAPABILITIES[\s\S]*?key:\s*'kanban'/)
  assert.match(source, /cluster\.slug/)
})

test('applyFulfilledValue 仅应用成功的辅助数据源', async () => {
  const [failed, succeeded] = await Promise.allSettled([
    Promise.reject(new Error('artifact unavailable')),
    Promise.resolve([{ id: 7 }]),
  ])
  const applied = []
  assert.equal(applyFulfilledValue(failed, (value) => applied.push(value)), false)
  assert.equal(applyFulfilledValue(succeeded, (value) => applied.push(value)), true)
  assert.deepEqual(applied, [[{ id: 7 }]])
})

test('成员请求控制器真实丢弃乱序响应与关闭后的响应', async () => {
  const gate = { current: 0 }
  const first = deferred()
  const second = deferred()
  const applied = []
  const settled = []
  const firstID = beginHermesRequest(gate)
  const firstRun = runLatestHermesRequest(gate, firstID, () => first.promise, (value) => applied.push(value), () => settled.push('first'))
  const secondID = beginHermesRequest(gate)
  const secondRun = runLatestHermesRequest(gate, secondID, () => second.promise, (value) => applied.push(value), () => settled.push('second'))
  second.resolve('B')
  assert.equal(await secondRun, true)
  first.resolve('A')
  assert.equal(await firstRun, false)
  assert.deepEqual(applied, ['B'])
  assert.deepEqual(settled, ['second'])

  const third = deferred()
  const thirdID = beginHermesRequest(gate)
  const thirdRun = runLatestHermesRequest(gate, thirdID, () => third.promise, (value) => applied.push(value), () => settled.push('third'))
  invalidateHermesRequests(gate)
  third.resolve('closed')
  assert.equal(await thirdRun, false)
  assert.deepEqual(applied, ['B'])
  assert.deepEqual(settled, ['second'])
})

test('fetchAllHermesPages 按页真实拉取到 total 收敛，不放大 page_size', async () => {
  const requested = []
  const all = Array.from({ length: 250 }, (_, i) => ({ id: i + 1 }))
  const result = await fetchAllHermesPages((page, pageSize) => {
    requested.push([page, pageSize])
    const start = (page - 1) * pageSize
    return Promise.resolve({ list: all.slice(start, start + pageSize), total: all.length })
  })
  assert.equal(result.length, 250)
  assert.deepEqual(requested, [[1, 100], [2, 100], [3, 100]])
})

test('fetchAllHermesPages 空页或 total 为 0 时提前终止，不无限循环', async () => {
  let calls = 0
  const empty = await fetchAllHermesPages(() => {
    calls += 1
    return Promise.resolve({ list: [], total: 0 })
  })
  assert.deepEqual(empty, [])
  assert.equal(calls, 1)

  let shrinkingCalls = 0
  const shrinking = await fetchAllHermesPages((page) => {
    shrinkingCalls += 1
    return page === 1
      ? Promise.resolve({ list: [{ id: 1 }], total: 5 })
      : Promise.resolve({ list: [], total: 5 })
  })
  assert.equal(shrinking.length, 1)
  assert.equal(shrinkingCalls, 2)
})

test('策略写入口与三个事件使用后端同款 policy:write 权限门控', async () => {
  const policies = await page('policies')
  assert.match(policies, /hasPerm\('ai:hermes:policy:write'\)/)
  assert.match(policies, /const openModal[\s\S]*?if \(!canWrite\) return[\s\S]*?const submit/)
  assert.match(policies, /const submit[\s\S]*?if \(!canWrite\) return[\s\S]*?const handleDelete/)
  assert.match(policies, /const handleDelete[\s\S]*?if \(!canWrite\) return[\s\S]*?const columns/)
  assert.equal((policies.match(/show: canWrite/g) ?? []).length, 2)
  assert.match(policies, /\{canWrite && \(/)
})

test('实例列表和成员抽屉共享四种状态的 i18n 词典', async () => {
  assert.deepEqual(HERMES_INSTANCE_STATUS_LABELS, {
    unassigned: '未放置',
    pending: '待启动',
    running: '运行中',
    error: '异常',
  })
  const clusters = await page('clusters')
  assert.match(clusters, /t\(HERMES_INSTANCE_STATUS_LABELS\[/)
})
