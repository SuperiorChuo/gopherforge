import { test, expect, type Page } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

// 两条独立必跑lane：默认production只注册整页验收；component显式选择且不加载登录态。
const lane = process.env.E2E_AGENT_EVENTS_LANE || 'production'
if (!['production', 'component'].includes(lane)) throw new Error('invalid Agent events test lane')
const componentLane = lane === 'component'
test.use({ storageState: componentLane ? { cookies: [], origins: [] } : process.env.E2E_STORAGE_STATE || STORAGE_STATE })

async function fixture(page: Page, failNext = false) {
  const requests: number[] = []
  let fail = failNext
  const run = { id: 810099, status: 'failed', input: '事件分页验收', output: '', attempt: 1, created_at: '2026-09-08T00:00:00Z', error_code: 'run_timeout' }
  // 只读浏览器投影，未列明的请求均拒绝，不向真实服务发Run或写数据。
  await page.route('**/api/v1/ai/agent-kernel/**', async route => {
    const url = new URL(route.request().url())
    if (route.request().method() !== 'GET') return route.abort('blockedbyclient')
    if (url.pathname.endsWith('/tools')) return route.fulfill({ json: { code: 200, data: [] } })
    if (url.pathname.endsWith('/agents')) return route.fulfill({ json: { code: 200, data: { list: [], total: 0 } } })
    if (url.pathname.endsWith('/runs')) return route.fulfill({ json: { code: 200, data: { list: [run], total: 1 } } })
    if (url.pathname.endsWith('/runs/810099')) {
      const after = Number(url.searchParams.get('after_id') || 0)
      requests.push(after)
      if (after && fail) { fail = false; return route.abort('failed') }
      const events = Array.from({ length: 401 }, (_, i) => ({ id: i + 1, run_id: run.id, type: `event.${i + 1}`, occurred_at: run.created_at }))
        .filter(event => event.id > after).slice(0, 200)
      const cursor = events.at(-1)?.id ?? after
      return route.fulfill({ json: { code: 200, data: { run, definition: { name: '分页助手' }, version: { version: 1 }, steps: [], tool_calls: [], events, events_next_cursor: cursor, events_has_more: cursor < 401 } } })
    }
    return route.abort('blockedbyclient')
  })
  await page.goto('/ai/agents')
  await page.getByText('810099', { exact: true }).click()
  await expect(page.getByRole('region', { name: '事件分页' }).getByText('event.1', { exact: true })).toBeVisible()
  return requests
}

if (!componentLane) for (const width of [1280, 390]) for (const theme of ['light', 'dark']) {
  test(`事件三批无丢失且DOM有界 ${width} ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.addInitScript(theme => localStorage.setItem('app_theme', theme), theme)
    const errors: string[] = []
    page.on('pageerror', () => errors.push('pageerror'))
    // 只记类型与无query的路由，禁止console原文把WebSocket鉴权参数带进报告。
    page.on('console', msg => { if (['error', 'warning'].includes(msg.type())) errors.push(msg.type()) })
    page.on('response', response => { if (response.status() >= 400) errors.push(`${response.status()} ${new URL(response.url()).pathname}`) })
    const requests = await fixture(page)
    const panel = page.getByRole('region', { name: '事件分页' })
    await expect(panel.locator('.ant-timeline-item')).toHaveCount(200)
    await panel.getByRole('button', { name: '下一批事件' }).click()
    await expect(panel.getByText('event.201', { exact: true })).toBeVisible()
    await expect(panel.getByText('event.1', { exact: true })).toHaveCount(0)
    await expect(panel.locator('.ant-timeline-item')).toHaveCount(200)
    await panel.getByRole('button', { name: '下一批事件' }).click()
    await expect(panel.getByText('event.401', { exact: true })).toBeVisible()
    await expect(panel.locator('.ant-timeline-item')).toHaveCount(1)
    await expect(panel.getByRole('button', { name: '下一批事件' })).toBeDisabled()
    await panel.getByRole('button', { name: '返回首批' }).click()
    await expect(panel.getByText('event.1', { exact: true })).toBeVisible()
    expect(requests.filter(cursor => cursor > 0)).toEqual([200, 400])
    const geometry = await page.evaluate(() => ({ body: document.body.scrollWidth <= document.body.clientWidth, root: document.documentElement.scrollWidth <= document.documentElement.clientWidth, theme: document.documentElement.dataset.theme }))
    expect(geometry).toEqual({ body: true, root: true, theme })
    expect(errors).toEqual([])
    await page.screenshot({ path: `.ui-evidence/agent-events-${width}-${theme}.png`, fullPage: true })
  })
}

// 独立组件测试使用真实RunEvents和API封装，不挂应用鉴权壳；不伪造已登录用户，
// 也不替代下方真实网关origin的整页验收。需以本地Vite为baseURL单独运行。
if (componentLane) test('组件尾批追加事件后可原位刷新且不退回首批', async ({ page, baseURL }) => {
  const componentURL = process.env.E2E_COMPONENT_BASE_URL || baseURL
  expect(componentURL).toMatch(/^http:\/\/127\.0\.0\.1:/)
  page.on('pageerror', error => console.log('component bootstrap:', error.message.replace(/https?:\/\/\S+|[A-Za-z0-9_-]{32,}/g, '[redacted]')))
  let count = 201
  const cursors: number[] = []
  const details = (after: number) => {
    const events = Array.from({ length: count }, (_, i) => ({ id: i + 1, run_id: 77, type: `event.${i + 1}`, occurred_at: '2026-09-08T00:00:00Z' })).filter(event => event.id > after).slice(0, 200)
    return { run: { id: 77, status: count > 201 ? 'failed' : 'running' }, events, events_has_more: (events.at(-1)?.id ?? after) < count, events_next_cursor: events.at(-1)?.id ?? after }
  }
  await page.route('**/api/v1/ai/agent-kernel/runs/77?*', route => {
    const cursor = Number(new URL(route.request().url()).searchParams.get('after_id'))
    cursors.push(cursor)
    return route.fulfill({ json: { code: 200, data: details(cursor) } })
  })
  await page.route('**/event-component-test', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><div id="root"></div><script type="module">
    const runtime = (await import('/@react-refresh')).default;
    runtime.injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type; window.__vite_plugin_react_preamble_installed__ = true;
    const React = (await import('/node_modules/.vite/deps/react.js')).default;
    const { createRoot } = (await import('/node_modules/.vite/deps/react-dom_client.js')).default;
    await import('/src/i18n/init.ts');
    const Component = (await import('/src/pages/ai/agents/run-events.tsx')).default;
    const root = createRoot(document.getElementById('root'));
    window.renderEvents = details => root.render(React.createElement(Component, {details}));
    window.renderEvents(${JSON.stringify(details(0))});
  </script>` }))
  await page.goto(new URL('/event-component-test', componentURL).href)
  const panel = page.getByRole('region', { name: '事件分页' })
  await panel.getByRole('button', { name: '下一批事件' }).click()
  await expect(panel.getByText('event.201', { exact: true })).toBeVisible()
  count = 203
  // 模拟父详情刷新到终态；当前批次仍须能主动探测新事件，不能只能返回首批重走。
  await page.evaluate(data => (window as unknown as { renderEvents: (data: unknown) => void }).renderEvents(data), details(0))
  await panel.getByRole('button', { name: '刷新当前批次' }).click()
  await expect(panel.getByText('event.203', { exact: true })).toBeVisible()
  await expect(panel.locator('.ant-timeline-item')).toHaveCount(3)
  await expect(panel.getByText('event.1', { exact: true })).toHaveCount(0)
  expect(cursors).toEqual([200, 200])
})

if (!componentLane) test('下一批失败保留当前事件且重试不跳游标', async ({ page }) => {
  const requests = await fixture(page, true)
  const panel = page.getByRole('region', { name: '事件分页' })
  await panel.getByRole('button', { name: '下一批事件' }).click()
  await expect(panel.getByText('事件加载失败，已保留当前批次')).toBeVisible()
  await expect(panel.getByText('event.1', { exact: true })).toBeVisible()
  await expect(panel.locator('.ant-timeline-item')).toHaveCount(200)
  await panel.getByRole('button', { name: /重\s*试/ }).click()
  await expect(panel.getByText('event.201', { exact: true })).toBeVisible()
  expect(requests.filter(cursor => cursor > 0)).toEqual([200, 200])
})
