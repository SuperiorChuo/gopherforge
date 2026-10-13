import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

// UI 打磨 A1 验收：bpm 三列表 × 1280/390 双档 + 截图 + console/pageerror 检查。
// 运行：E2E_BASE_URL=http://localhost:8000 npx playwright test bpm-ui-audit
const PAGES = [
  {
    path: '/bpm/definitions',
    marker: '.bpm-definitions-page .list-table',
    dataPath: '/api/v1/bpm/definitions',
  },
  {
    path: '/bpm/instances',
    marker: '.bpm-instances-page .list-table',
    dataPath: '/api/v1/bpm/instances/my',
  },
  {
    path: '/bpm/tasks',
    marker: '.bpm-tasks-page .list-table',
    dataPath: '/api/v1/bpm/tasks/todo',
  },
]

async function audit(
  page: import('@playwright/test').Page,
  target: (typeof PAGES)[number],
  screenshotPath: string,
) {
  const errs: string[] = []
  const pendingAPIRequests = new Set<import('@playwright/test').Request>()
  const isAPIRequest = (request: import('@playwright/test').Request) =>
    request.resourceType() === 'fetch' || request.resourceType() === 'xhr'
  page.on('console', (message) => {
    if (message.type() !== 'error' && message.type() !== 'warning') return
    errs.push(`CONSOLE-${message.type().toUpperCase()}: ${message.text().slice(0, 120)}`)
  })
  page.on('pageerror', (error) => errs.push(`PAGEERROR: ${String(error).slice(0, 120)}`))
  page.on('request', (request) => {
    if (isAPIRequest(request)) pendingAPIRequests.add(request)
  })
  page.on('requestfinished', (request) => pendingAPIRequests.delete(request))
  page.on('requestfailed', (request) => {
    pendingAPIRequests.delete(request)
    if (isAPIRequest(request)) {
      errs.push(`REQUEST-FAILED: ${request.url().slice(0, 120)}`)
    }
  })
  page.on('response', (response) => {
    if (isAPIRequest(response.request()) && response.status() >= 400) {
      errs.push(`HTTP-${response.status()}: ${response.url().slice(0, 120)}`)
    }
  })

  const dataResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname === target.dataPath,
  )
  const [response, dataResponse] = await Promise.all([
    page.goto(target.path),
    dataResponsePromise,
  ])
  expect(response?.ok(), `${target.path} navigation response`).toBeTruthy()
  expect(dataResponse.ok(), `${target.dataPath} data response`).toBeTruthy()
  expect(new URL(page.url()).pathname, `${target.path} final pathname`).toBe(target.path)
  await expect(page.locator(target.marker).first(), `${target.path} list marker`).toBeVisible()
  await expect(page.locator(`${target.marker} .ant-spin-spinning`), `${target.path} loading`).toHaveCount(0)
  await expect.poll(
    () => pendingAPIRequests.size,
    { message: `${target.path} pending API requests`, timeout: 10_000 },
  ).toBe(0)
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))

  const report = await page.evaluate(() => {
    const issues: string[] = []
    for (const [name, root] of [
      ['HTML', document.documentElement],
      ['BODY', document.body],
    ] as const) {
      if (root.scrollWidth > root.clientWidth) {
        issues.push(`${name}-OVERFLOW scrollWidth=${root.scrollWidth} clientWidth=${root.clientWidth}`)
      }
    }
    const leaks = document.body.innerHTML.match(/\{\{[^}]+\}\}|undefined|NaN/g) ?? []
    if (leaks.length) issues.push(`MARKER-LEAK: ${leaks.slice(0, 3).join(' ')}`)
    return issues
  })

  await page.screenshot({ path: screenshotPath, fullPage: true })
  return [...report, ...errs]
}

test('bpm 三列表 1280/390 双档验收', async ({ browser }, testInfo) => {
  for (const viewport of [
    { width: 1280, height: 800 },
    { width: 390, height: 844 },
  ]) {
    const context = await browser.newContext({
      viewport,
      storageState: STORAGE_STATE,
    })

    for (const target of PAGES) {
      const page = await context.newPage()
      const pageSlug = target.path.slice(1).replaceAll('/', '-')
      const issues = await audit(
        page,
        target,
        testInfo.outputPath(`bpm-${pageSlug}-${viewport.width}.png`),
      )
      await page.close()
      expect(issues, `${target.path} ${viewport.width}px`).toEqual([])
    }

    await context.close()
  }
})
