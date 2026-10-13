import { test, expect, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

// 可显式复用人工 TOTP 登录态；不把验证码/凭据写入测试、截图或断言输出。
test.use({ storageState: process.env.E2E_STORAGE_STATE || STORAGE_STATE })

async function fixture(page: Page, options: { permissions?: string[]; failLoad?: boolean; failPatch?: boolean; status?: string; wrongId?: boolean } = {}) {
  let definition = { id: 810001, tenant_id: 1, agent_key: 'editor_fixture', name: '定义编辑回归', description: '原描述', status: options.status ?? 'active', latest_version_id: 810009, created_at: '2026-09-08T00:00:00Z' }
  const patches: unknown[] = []
  let loads = 0
  let loadFails = options.failLoad ?? false
  // 身份请求仍经过真实登录；这里只改变浏览器权限投影，不冒充服务端RBAC验收。
  if (options.permissions) await page.route('**/api/v1/user/me', async route => {
    const response = await route.fetch()
    const payload = await response.json()
    expect(response.status()).toBe(200)
    await route.fulfill({ json: { ...payload, data: { ...payload.data, roles: [], permissions: options.permissions } } })
  })
  // 整个Kernel命名空间只在浏览器内模拟，任何未声明写请求都失败，不污染线上数据。
  await page.route('**/api/v1/ai/agent-kernel/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const pathname = url.pathname
    if (request.method() === 'GET' && pathname.endsWith('/agents')) return route.fulfill({ json: { code: 200, data: { list: [definition], total: 1 } } })
    if (request.method() === 'GET' && pathname.endsWith('/runs')) return route.fulfill({ json: { code: 200, data: { list: [], total: 0 } } })
    if (request.method() === 'GET' && pathname.endsWith('/agents/810001')) {
      loads++
      if (loadFails) return route.abort('failed')
      return route.fulfill({ json: { code: 200, data: { definition: options.wrongId ? { ...definition, id: 810002 } : definition, versions: [] } } })
    }
    if (request.method() === 'PATCH' && pathname.endsWith('/agents/810001')) {
      const body = request.postDataJSON()
      patches.push(body)
      if (options.failPatch && patches.length === 1) return route.abort('failed')
      definition = { ...definition, ...body }
      return route.fulfill({ json: { code: 200, data: definition } })
    }
    return route.abort('blockedbyclient')
  })
  await page.goto('/ai/agents')
  await expect(page.getByText('定义编辑回归', { exact: true })).toBeVisible()
  return { patches, loads: () => loads, allowLoad: () => { loadFails = false } }
}

async function edit(page: Page) {
  await page.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '编辑 Agent', exact: true })).toBeVisible()
}

async function save(page: Page) {
  await page.getByRole('dialog', { name: '编辑 Agent', exact: true }).getByRole('button', { name: /确\s*定/ }).click()
}

async function selectStatus(page: Page, label: string) {
  await page.getByRole('combobox', { name: /状态$/ }).click()
  // Ant Select 的ARIA option是虚拟辅助节点；点击实际可见选项，不强制点击隐藏节点。
  await page.locator('.ant-select-dropdown:visible').getByText(label, { exact: true }).click()
}

test('编辑白名单字段与禁用/启用闭环，禁用不提供新运行入口', async ({ page }) => {
  const state = await fixture(page)
  await edit(page)
  await expect(page.getByLabel('Agent 标识', { exact: true })).toBeDisabled()
  await page.getByRole('textbox', { name: /名称$/ }).fill('修改后名称')
  await page.getByRole('textbox', { name: '描述', exact: true }).fill('')
  await selectStatus(page, '禁用')
  await save(page)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(state.patches).toEqual([{ name: '修改后名称', description: '', status: 'disabled' }])
  await expect(page.getByText('修改后名称', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '更多操作', exact: true }).click()
  await expect(page.getByRole('menuitem', { name: /运行/ })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await edit(page)
  await selectStatus(page, '启用')
  await save(page)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(state.patches[1]).toEqual({ name: '修改后名称', description: '', status: 'active' })
  await page.getByRole('button', { name: '更多操作', exact: true }).click()
  await expect(page.getByRole('menuitem', { name: /运行/ })).toBeVisible()
})

test('定义读取失败禁用保存，重试必须重新取得正确定义', async ({ page }) => {
  const state = await fixture(page, { failLoad: true })
  await edit(page)
  await expect(page.getByText('Agent 定义加载失败', { exact: true })).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('button', { name: /确\s*定/ })).toBeDisabled()
  expect(state.patches).toEqual([])
  // 开发StrictMode允许effect双调用；故障持续到人工重试，不用第几次请求模拟环境。
  const beforeRetry = state.loads()
  state.allowLoad()
  await page.getByRole('button', { name: /重\s*试/ }).click()
  await expect(page.getByRole('textbox', { name: /名称$/ })).toHaveValue('定义编辑回归')
  expect(state.loads()).toBe(beforeRetry + 1)
})

test('错误ID的定义响应不能保存到当前目标', async ({ page }) => {
  const state = await fixture(page, { wrongId: true })
  await edit(page)
  await expect(page.getByText('Agent 定义加载失败', { exact: true })).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('button', { name: /确\s*定/ })).toBeDisabled()
  expect(state.patches).toEqual([])
})

test('保存失败不关窗、不丢用户输入，重试使用同一白名单内容', async ({ page }) => {
  const state = await fixture(page, { failPatch: true })
  await edit(page)
  await page.getByRole('textbox', { name: /名称$/ }).fill('保留输入')
  await save(page)
  await expect.poll(() => state.patches.length).toBe(1)
  await expect(page.getByRole('textbox', { name: /名称$/ })).toHaveValue('保留输入')
  await expect(page.getByRole('dialog').getByRole('button', { name: /确\s*定/ })).toBeEnabled()
  await save(page)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(state.patches).toEqual([{ name: '保留输入', description: '原描述', status: 'active' }, { name: '保留输入', description: '原描述', status: 'active' }])
})

test('只读权限没有编辑、新建或运行入口', async ({ page }) => {
  const state = await fixture(page, { permissions: ['ai:agent:list'] })
  await expect(page.getByRole('button', { name: '编辑', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /新建 Agent/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '更多操作', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '运行', exact: true })).toHaveCount(0)
  expect(state.patches).toEqual([])
})

for (const status of ['active', 'disabled']) test(`仅运行权限在${status}状态正确显示入口（含更多菜单）`, async ({ page }) => {
  await fixture(page, { permissions: ['ai:agent:list', 'ai:agent:run'], status })
  await expect(page.getByRole('button', { name: '编辑', exact: true })).toHaveCount(0)
  const more = page.getByRole('button', { name: '更多操作', exact: true })
  if (await more.count()) await more.click()
  const run = page.getByRole('button', { name: '运行', exact: true }).or(page.getByRole('menuitem', { name: /运行/ }))
  if (status === 'active') await expect(run).toBeVisible()
  else await expect(run).toHaveCount(0)
})

test('旧GET挂起后关闭重开，迟到响应不覆盖新表单', async ({ page }) => {
  await fixture(page)
  const pending: Route[] = []
  let hold = true
  const response = (name: string) => ({ code: 200, data: { definition: { id: 810001, agent_key: 'editor_fixture', name, description: '', status: 'active' }, versions: [] } })
  await page.route('**/api/v1/ai/agent-kernel/agents/810001', async route => {
    if (route.request().method() !== 'GET') return route.fallback()
    if (hold) { pending.push(route); return }
    await route.fulfill({ json: response('新会话名称') })
  })
  await edit(page)
  await expect.poll(() => pending.length).toBeGreaterThan(0)
  await page.getByRole('dialog').getByRole('button', { name: /取\s*消/ }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  hold = false
  await edit(page)
  await expect(page.getByRole('textbox', { name: /名称$/ })).toHaveValue('新会话名称')
  // 先注册旧响应的消费完成监听，再释放网络闸门；不用固定睡眠冒充请求已处理。
  const consumed = pending.map(route => new Promise<void>(resolve => {
    const listener = async (finished: import('@playwright/test').Request) => {
      if (finished !== route.request()) return
      page.off('requestfinished', listener)
      resolve()
    }
    page.on('requestfinished', listener)
  }))
  await Promise.all(pending.map(route => route.fulfill({ json: response('旧响应不得覆盖') })))
  await Promise.all(consumed)
  // 网络完成后跨过渲染帧，让已排队的Promise/React更新先执行；只证明关窗重开用户路径，
  // 不宣称此路径单独证明requestID保护（父组件卸载隔离也是必要前提）。
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect(page.getByRole('textbox', { name: /名称$/ })).toHaveValue('新会话名称')
})

test('PATCH挂起期间同轮重复提交只产生一次请求且不能关闭重开', async ({ page }) => {
  await fixture(page)
  const pending: Route[] = []
  await page.route('**/api/v1/ai/agent-kernel/agents/810001', async route => {
    if (route.request().method() !== 'PATCH') return route.fallback()
    pending.push(route)
  })
  await edit(page)
  await expect(page.getByRole('textbox', { name: /名称$/ })).toHaveValue('定义编辑回归')
  const dialog = page.getByRole('dialog')
  // 两次同事件循环点击能拒绝仅靠异步saving state、没有同步互斥的错误实现。
  await dialog.getByRole('button', { name: /确\s*定/ }).evaluate((element: HTMLButtonElement) => { element.click(); element.click() })
  await expect.poll(() => pending.length).toBe(1)
  await expect(dialog.getByRole('button', { name: /取\s*消/ })).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  expect(pending).toHaveLength(1)
  expect(pending[0].request().postDataJSON()).toEqual({ name: '定义编辑回归', description: '原描述', status: 'active' })
  await pending[0].fulfill({ json: { code: 200, data: { id: 810001, name: '定义编辑回归', description: '原描述', status: 'active' } } })
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('迟到B详情与A保存交错，不拼接不同定义的版本列表', async ({ page }) => {
  await fixture(page)
  const a = { id: 810001, name: '定义A', agent_key: 'definition_a', description: '', status: 'active', latest_version_id: 810009 }
  const b = { ...a, id: 810002, name: '定义B', agent_key: 'definition_b' }
  const pendingB: Route[] = []
  const pendingPatch: Route[] = []
  await page.route('**/api/v1/ai/agent-kernel/agents?*', route => route.fulfill({ json: { code: 200, data: { list: [a, b], total: 2 } } }))
  await page.route('**/api/v1/ai/agent-kernel/agents/810001', route => {
    if (route.request().method() === 'PATCH') { pendingPatch.push(route); return }
    return route.fulfill({ json: { code: 200, data: { definition: a, versions: [] } } })
  })
  await page.route('**/api/v1/ai/agent-kernel/agents/810002', route => { pendingB.push(route) })
  await page.reload()
  const details = page.getByRole('button', { name: '详情', exact: true })
  await details.first().click()
  await expect(page.getByRole('dialog', { name: 'Agent 详情 · 定义A', exact: true })).toBeVisible()
  await page.locator('.ant-drawer-close').click()
  await details.nth(1).click()
  await expect.poll(() => pendingB.length).toBe(1)
  await page.locator('.ant-drawer-close').click()
  await page.getByRole('button', { name: '编辑', exact: true }).first().click()
  await page.getByRole('textbox', { name: /名称$/ }).fill('修改后的A')
  await save(page)
  await expect.poll(() => pendingPatch.length).toBe(1)
  const responseB = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/agents/810002'))
  await pendingB[0].fulfill({ json: { code: 200, data: { definition: b, versions: [] } } })
  await (await responseB).finished()
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await pendingPatch[0].fulfill({ json: { code: 200, data: { ...a, name: '修改后的A' } } })
  await expect(page.getByRole('dialog', { name: '编辑 Agent', exact: true })).toHaveCount(0)
  // 再开B但挂起其新GET，直接核对当前缓存，不能让新响应掩盖混写。
  await details.nth(1).click()
  await expect.poll(() => pendingB.length).toBe(2)
  await expect(page.getByRole('dialog', { name: 'Agent 详情 · 定义B', exact: true })).toBeVisible()
})

test('全空白名称拒绝发请求，取消编辑没有PATCH副作用', async ({ page }) => {
  const state = await fixture(page)
  await edit(page)
  await page.getByRole('textbox', { name: /名称$/ }).fill('   ')
  await save(page)
  await expect(page.getByText('请输入名称', { exact: true })).toBeVisible()
  expect(state.patches).toEqual([])
  await page.getByRole('dialog').getByRole('button', { name: /取\s*消/ }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(state.patches).toEqual([])
})
