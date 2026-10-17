import { test, expect, type Page, type Route } from '@playwright/test'

// 组件 lane：以本地 Vite 为 baseURL，直接挂 /ai/agents 整页，不挂应用鉴权壳、不需要登录态。
// 与 agent-events 的组件 lane 同一思路：只验证浏览器内的竞态与状态完备，
// 不冒充真实网关 origin 的整页验收（那条走 production lane）。
// 使用组件配置启动的 Vite baseURL，不依赖固定端口或完整栈的静态 nginx。

const definitionA = {
  id: 910001, tenant_id: 1, agent_key: 'definition_a', name: '定义A',
  description: '', status: 'active', latest_version_id: 910009, created_at: '2026-09-08T00:00:00Z',
}
const definitionB = { ...definitionA, id: 910002, agent_key: 'definition_b', name: '定义B' }

async function mountPage(page: Page, pending: Record<number, Route[]>, options: { failDetail?: number[]; permissions?: string[] } = {}) {
  // 组件 lane 没有登录态；需要写权限的入口（发布版本）必须显式投影权限，否则按钮不渲染。
  if (options.permissions) {
    await page.route('**/api/v1/user/me', route => route.fulfill({
      json: { code: 200, data: { id: 1, username: 'e2e', roles: [], permissions: options.permissions } },
    }))
  }
  await page.route('**/api/v1/ai/models/enabled', route => route.fulfill({ json: { code: 200, data: [] } }))
  await page.route('**/api/v1/ai/agent-kernel/tools', route => route.fulfill({ json: { code: 200, data: [] } }))
  await page.route('**/api/v1/ai/agent-kernel/runs*', route => route.fulfill({ json: { code: 200, data: { list: [], total: 0 } } }))
  await page.route('**/api/v1/ai/agent-kernel/agents?*', route =>
    route.fulfill({ json: { code: 200, data: { list: [definitionA, definitionB], total: 2 } } }))
  for (const id of [910001, 910002]) {
    await page.route(`**/api/v1/ai/agent-kernel/agents/${id}`, route => {
      if (options.failDetail?.includes(id)) return route.fulfill({ status: 500, json: { code: 500, message: 'boom' } })
      pending[id] = pending[id] || []
      pending[id].push(route)
    })
  }
  await page.route('**/agent-detail-component', route => route.fulfill({
    contentType: 'text/html',
    body: `<!doctype html><div id="root"></div><script type="module">
    const runtime = (await import('/@react-refresh')).default;
    runtime.injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type; window.__vite_plugin_react_preamble_installed__ = true;
    const React = (await import('/node_modules/.vite/deps/react.js')).default;
    const { createRoot } = (await import('/node_modules/.vite/deps/react-dom_client.js')).default;
    const { Provider } = (await import('/node_modules/.vite/deps/react-redux.js'));
    const { store } = await import('/src/store/index.ts');
    // 组件 lane 不走应用 bootstrap，权限必须直接进 store；否则依赖写权限的入口不渲染。
    store.dispatch({ type: 'auth/login/fulfilled', payload: { user: { permissions: ${JSON.stringify(options.permissions ?? [])} } } });
    await import('/src/i18n/init.ts');
    const Component = (await import('/src/pages/ai/agents/index.tsx')).default;
    // 整页依赖 redux（子组件用 useAppSelector），组件 lane 必须自备 Provider。
    createRoot(document.getElementById('root')).render(
      React.createElement(Provider, { store }, React.createElement(Component))
    );
  </script>`,
  }))
  await page.goto('/agent-detail-component')
  const rows = page.getByRole('button', { name: '详情', exact: true })
  await expect(rows).toHaveCount(2)
  return rows
}

test('两个详情GET乱序返回时以最后点击的定义为准，迟到响应不得覆盖', async ({ page }) => {
  const pending: Record<number, Route[]> = {}
  const rows = await mountPage(page, pending)

  // 点 A 的详情：请求挂起
  await rows.first().click()
  await expect.poll(() => pending[910001]?.length ?? 0).toBe(1)
  await page.locator('.ant-drawer-close').click()

  // 点 B 的详情并让它先返回
  await rows.nth(1).click()
  await expect.poll(() => pending[910002]?.length ?? 0).toBe(1)
  await pending[910002][0].fulfill({ json: { code: 200, data: { definition: definitionB, versions: [] } } })
  await expect(page.getByRole('dialog', { name: 'Agent 详情 · 定义B', exact: true })).toBeVisible()

  // A 迟到返回：必须被丢弃，抽屉仍是 B
  await pending[910001][0].fulfill({ json: { code: 200, data: { definition: definitionA, versions: [] } } })
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect(page.getByRole('dialog', { name: 'Agent 详情 · 定义B', exact: true })).toBeVisible()
  await expect(page.getByRole('dialog', { name: /定义A/ })).toHaveCount(0)
})

test('切换不同定义时不得展示上一个定义的版本列表', async ({ page }) => {
  const pending: Record<number, Route[]> = {}
  const rows = await mountPage(page, pending)

  // A 正常返回并展示
  await rows.first().click()
  await expect.poll(() => pending[910001]?.length ?? 0).toBe(1)
  await pending[910001][0].fulfill({ json: { code: 200, data: { definition: definitionA, versions: [{ id: 910009, version: 1, model_id: 2, tools: [], max_steps: 8, timeout_seconds: 120, created_at: '2026-09-08T00:00:00Z' }] } } })
  await expect(page.getByRole('dialog', { name: 'Agent 详情 · 定义A', exact: true })).toBeVisible()
  await page.locator('.ant-drawer-close').click()

  // 再开 B：其请求挂起期间，绝不能继续显示 A 的版本行
  await rows.nth(1).click()
  await expect.poll(() => pending[910002]?.length ?? 0).toBe(1)
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect(page.getByRole('dialog', { name: /定义A/ })).toHaveCount(0)
})

test('运行选项加载失败时显示可重试错误，而不是"暂无已启用模型"的假空值', async ({ page }) => {
  const pending: Record<number, Route[]> = {}
  await mountPage(page, pending, { permissions: ['ai:agent:write', 'ai:agent:list'] })
  // 必须在 mountPage 之后覆盖：Playwright 后注册的路由优先，先注册会被默认 200 掩盖。
  await page.route('**/api/v1/ai/models/enabled', route => route.fulfill({ status: 500, json: { code: 500, message: 'boom' } }))

  // 打开发布版本弹窗：模型列表拉取失败（该入口在行内「更多操作」菜单里）
  await page.getByRole('button', { name: '更多操作' }).first().click()
  await page.getByRole('menuitem', { name: '发布版本' }).click()
  // notFoundContent 只在 Select 下拉展开时渲染。
  await page.getByRole('combobox', { name: '* 模型' }).click()
  await expect(page.getByText('模型加载失败')).toBeVisible({ timeout: 5000 })
  await expect(page.getByRole('button', { name: /重\s*试/ }).first()).toBeVisible()
  // 服务故障绝不能被渲染成"确实没有模型"（假零值），那会引导用户去配置页白找。
  await expect(page.getByText('暂无已启用模型，请先到「AI 模型配置」添加')).toHaveCount(0)
})

test('详情加载失败时显示可重试的错误态，而不是空白', async ({ page }) => {
  const pending: Record<number, Route[]> = {}
  const rows = await mountPage(page, pending, { failDetail: [910001] })

  await rows.first().click()
  await expect(page.getByText('详情加载失败')).toBeVisible()
  await expect(page.getByRole('button', { name: /重\s*试/ })).toBeVisible()
})
