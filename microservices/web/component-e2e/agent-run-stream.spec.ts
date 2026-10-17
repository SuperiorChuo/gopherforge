import { test, expect, type Page } from '@playwright/test'

// 组件 lane：本地 Vite 挂 RunDrawer，不需要登录态。用于验证事件流驱动的两个行为：
// ① 逐帧刷新必须被节流（后端单轮最多推 200 条事件）；
// ② 流"正常结束"（干净 EOF、无 done 帧）也必须收尾，不能永远停在"实时跟踪中"。
// Vite 由组件配置管理，目标 origin 使用该配置的 baseURL。

const RUN_ID = 920001

function detailsPayload(events: Array<{ id: number; type: string }>, status = 'running') {
  return {
    run: { id: RUN_ID, status, definition_id: 1, version_id: 1, requested_by: 1, input: 'x', error_message: '', created_at: '2026-09-08T00:00:00Z' },
    definition: { id: 1, name: '定义' },
    version: { id: 1, version: 1 },
    steps: [],
    events: events.map(event => ({ ...event, run_id: RUN_ID, occurred_at: '2026-09-08T00:00:00Z' })),
    events_has_more: false,
    events_next_cursor: events.at(-1)?.id ?? 0,
  }
}

async function mountDrawer(page: Page, options: { frames: number; endWithoutDone: boolean; detailRequests: number[]; streamDelayMs?: number }) {
  await page.route(`**/api/v1/ai/agent-kernel/runs/${RUN_ID}?*`, route => {
    options.detailRequests.push(Number(new URL(route.request().url()).searchParams.get('after_id')))
    return route.fulfill({ json: { code: 200, data: detailsPayload([{ id: 1, type: 'run.queued' }]) } })
  })
  await page.route(`**/api/v1/ai/agent-kernel/runs/${RUN_ID}/events*`, async route => {
    // 延迟返回：让"已开始跟踪"这一中间态可被观测，避免流太快导致断言竞态。
    if (options.streamDelayMs) await new Promise(resolve => setTimeout(resolve, options.streamDelayMs))
    // 一次给出 N 条数据帧；endWithoutDone 时**不发** done 帧，模拟网关干净收流。
    const frames = Array.from({ length: options.frames }, (_, i) =>
      `data: ${JSON.stringify({ id: i + 2, run_id: RUN_ID, type: `run.step.${i + 2}`, occurred_at: '2026-09-08T00:00:00Z' })}\n\n`).join('')
    const tail = options.endWithoutDone ? '' : 'event: done\ndata: {"run_id":920001}\n\n'
    return route.fulfill({ status: 200, headers: { 'Content-Type': 'text/event-stream' }, body: frames + tail })
  })
  await page.route('**/agent-run-component', route => route.fulfill({
    contentType: 'text/html',
    body: `<!doctype html><div id="root"></div><script type="module">
    const runtime = (await import('/@react-refresh')).default;
    runtime.injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type; window.__vite_plugin_react_preamble_installed__ = true;
    const React = (await import('/node_modules/.vite/deps/react.js')).default;
    const { createRoot } = (await import('/node_modules/.vite/deps/react-dom_client.js')).default;
    const { Provider } = (await import('/node_modules/.vite/deps/react-redux.js'));
    const { store } = await import('/src/store/index.ts');
    await import('/src/i18n/init.ts');
    const Drawer = (await import('/src/pages/ai/agents/run-drawer.tsx')).default;
    createRoot(document.getElementById('root')).render(
      React.createElement(Provider, { store },
        React.createElement(Drawer, { runId: ${RUN_ID}, open: true, onClose: () => {} }))
    );
  </script>`,
  }))
  await page.goto('/agent-run-component')
  await expect(page.getByRole('dialog')).toBeVisible()
}

test('逐帧事件不得各自触发一次详情请求（请求风暴）', async ({ page }) => {
  const detailRequests: number[] = []
  await mountDrawer(page, { frames: 50, endWithoutDone: false, detailRequests })
  // 等流跑完（done 分支会再刷一次）
  await page.waitForTimeout(2500)
  // 未节流时这里会是 50+ 次；节流后应是个位数（首帧 + 尾沿 + done 各一次量级）。
  expect(detailRequests.length).toBeLessThan(6)
})

test('流正常结束（无 done 帧）也必须停止实时跟踪', async ({ page }) => {
  const detailRequests: number[] = []
  await mountDrawer(page, { frames: 3, endWithoutDone: true, detailRequests, streamDelayMs: 1200 })
  // 这一步同时是防空洞断言：能观察到"跟踪中"才证明流真的开起来了。
  await expect(page.getByText('实时跟踪中…')).toBeVisible()
  // 干净 EOF 后必须收尾；只挂 .catch 的实现会永远停在"实时跟踪中"。
  await expect(page.getByText('未连接实时流')).toBeVisible({ timeout: 5000 })
})
