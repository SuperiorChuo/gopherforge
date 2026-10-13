import { expect, test, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const account = {
  id: 101,
  tenant_id: 1,
  app_id: 'wx-e2e-fans',
  name: 'E2E 粉丝服务号',
  type: 'service',
  gh_id: 'gh_e2e_fans',
  secret_ref: 'MP_E2E_SECRET',
  aes_key_ref: '',
  encrypt_mode: 'plain',
  auto_publish: false,
  status: 'active',
  created_at: '2026-08-03T08:00:00Z',
  updated_at: '2026-08-03T08:00:00Z',
}

const secondAccount = {
  ...account,
  id: 102,
  app_id: 'wx-e2e-fans-second',
  name: 'E2E 第二公众号',
  gh_id: 'gh_e2e_fans_second',
  secret_ref: 'MP_E2E_SECOND_SECRET',
}

const fan = {
  id: 701,
  account_id: account.id,
  open_id: 'openid-e2e-fan-701',
  nickname: '同步后的粉丝',
  avatar: '',
  city: '上海',
  tags: '101,102',
  subscribed: true,
  subscribe_time: '2026-08-03T08:30:00Z',
  created_at: '2026-08-03T08:30:00Z',
}

type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed'

type FanSyncJob = {
  id: number
  account_id: number
  status: JobStatus
  total_count: number
  listed_count: number
  synced_count: number
  attempts: number
  last_error?: string
  next_at?: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

function success(data: unknown) {
  return { code: 200, message: 'success', data }
}

async function fulfill(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(success(data)),
  })
}

function createJob(status: JobStatus, accountId = account.id): FanSyncJob {
  const terminal = status === 'succeeded' || status === 'failed'
  return {
    id: 801,
    account_id: accountId,
    status,
    total_count: status === 'pending' ? 0 : 2,
    listed_count: status === 'pending' ? 0 : 2,
    synced_count: status === 'pending' ? 0 : status === 'running' ? 1 : 2,
    attempts: status === 'pending' ? 0 : 1,
    last_error: status === 'failed' ? '模拟微信接口失败' : undefined,
    started_at: status === 'pending' ? undefined : '2026-08-03T09:00:02Z',
    finished_at: terminal ? '2026-08-03T09:00:06Z' : undefined,
    created_at: '2026-08-03T09:00:00Z',
    updated_at: terminal ? '2026-08-03T09:00:06Z' : '2026-08-03T09:00:02Z',
  }
}

async function installFanRoutes(page: Page, initialStatus?: JobStatus) {
  let job = initialStatus ? createJob(initialStatus) : null
  let completed = initialStatus === 'succeeded'
  let holdRunning = initialStatus === 'running'
  let jobReads = 0
  let fanListReads = 0
  let syncStarts = 0

  await page.route('**/api/v1/mp/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const method = request.method()

    if (path === '/api/v1/mp/accounts' && method === 'GET') {
      await fulfill(route, { list: [account], total: 1, page: 1, page_size: 100 })
      return
    }
    if (path === '/api/v1/mp/fans' && method === 'GET') {
      fanListReads += 1
      const list = completed ? [fan] : []
      await fulfill(route, { list, total: list.length, page: 1, page_size: 10 })
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/fans/sync` && method === 'POST') {
      syncStarts += 1
      job = createJob('pending')
      completed = false
      await fulfill(route, { job, reused: false }, 202)
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/fans/sync` && method === 'GET') {
      if (job) {
        jobReads += 1
        if (job.status === 'pending') {
          job = createJob('running')
        } else if (job.status === 'running') {
          if (!holdRunning) {
            job = createJob('succeeded')
            completed = true
          }
        }
      }
      await fulfill(route, { job })
      return
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ code: 404, message: `unexpected ${method} ${path}` }),
    })
  })

  return {
    getFanListReads: () => fanListReads,
    getJobReads: () => jobReads,
    getSyncStarts: () => syncStarts,
    releaseRunning: () => { holdRunning = false },
  }
}

async function installAccountSwitchRoutes(page: Page) {
  let syncStarts = 0
  let releaseSecondStatus = () => {}
  let markSecondStatusRequested = () => {}
  const secondStatusGate = new Promise<void>((resolve) => { releaseSecondStatus = () => resolve() })
  const secondStatusRequested = new Promise<void>((resolve) => { markSecondStatusRequested = () => resolve() })

  await page.route('**/api/v1/mp/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const { pathname: path } = url
    const method = request.method()

    if (path === '/api/v1/mp/accounts' && method === 'GET') {
      await fulfill(route, { list: [account, secondAccount], total: 2, page: 1, page_size: 100 })
      return
    }
    if (path === '/api/v1/mp/fans' && method === 'GET') {
      await fulfill(route, { list: [], total: 0, page: 1, page_size: 10 })
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/fans/sync` && method === 'GET') {
      await fulfill(route, { job: null })
      return
    }
    if (path === `/api/v1/mp/accounts/${secondAccount.id}/fans/sync` && method === 'GET') {
      markSecondStatusRequested()
      await secondStatusGate
      await fulfill(route, { job: null })
      return
    }
    if (path.endsWith('/fans/sync') && method === 'POST') {
      syncStarts += 1
      await fulfill(route, { job: createJob('pending', secondAccount.id), reused: false }, 202)
      return
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ code: 404, message: `unexpected ${method} ${path}` }),
    })
  })

  return {
    getSyncStarts: () => syncStarts,
    releaseSecondStatus,
    secondStatusRequested,
  }
}

async function installReadOnlyUserRoutes(page: Page) {
  await page.route('**/api/v1/user/me', async (route) => {
    await fulfill(route, {
      id: 902,
      tenant_id: 1,
      username: 'mp-reader',
      nickname: '公众号只读员',
      status: 1,
      roles: [{ id: 9, name: '公众号只读', code: 'mp_reader' }],
      permissions: ['social:read'],
      must_change_password: false,
    })
  })
  await page.route('**/api/v1/user/menus', async (route) => fulfill(route, []))
}

async function installDeepAccountRoutes(page: Page) {
  await page.route('**/api/v1/mp/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const method = request.method()
    if (path === '/api/v1/mp/accounts' && method === 'GET') {
      await fulfill(route, { list: [account], total: 101, page: 1, page_size: 100 })
      return
    }
    if (path === `/api/v1/mp/accounts/${secondAccount.id}` && method === 'GET') {
      await fulfill(route, secondAccount)
      return
    }
    if (path === '/api/v1/mp/fans' && method === 'GET') {
      await fulfill(route, { list: [], total: 0, page: 1, page_size: 10 })
      return
    }
    if (path === `/api/v1/mp/accounts/${secondAccount.id}/fans/sync` && method === 'GET') {
      await fulfill(route, { job: null })
      return
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ code: 404, message: `unexpected ${method} ${path}` }),
    })
  })
}

test.describe('公众号 M4-B 粉丝后台同步', () => {
  test('任务从等待、运行到完成后自动刷新粉丝列表', async ({ page }) => {
    const state = await installFanRoutes(page)
    await page.goto(`/mp/fans?account_id=${account.id}`)

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('尚未同步粉丝资料')
    const startButton = page.getByRole('button', { name: '同步粉丝' })
    await expect(startButton).toBeEnabled()
    await startButton.click()

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('等待执行')
    await expect(page.getByRole('button', { name: '同步中' })).toBeDisabled()
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步中', { timeout: 6_000 })
    await expect(page.locator('.mp-fans-sync-alert').getByRole('progressbar')).toHaveAttribute('aria-valuenow', '50')
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步完成', { timeout: 8_000 })
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('已写入2')
    await expect(page.getByText(fan.nickname)).toBeVisible()

    expect(state.getSyncStarts()).toBe(1)
    expect(state.getJobReads()).toBeGreaterThanOrEqual(2)
    expect(state.getFanListReads()).toBeGreaterThanOrEqual(2)

    const terminalReads = state.getJobReads()
    await page.waitForTimeout(2_500)
    expect(state.getJobReads()).toBe(terminalReads)
  })

  test('只读权限不渲染同步命令', async ({ page }) => {
    await installReadOnlyUserRoutes(page)
    await installFanRoutes(page)
    await page.goto(`/mp/fans?account_id=${account.id}`)

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('尚未同步粉丝资料')
    await expect(page.getByRole('button', { name: '同步粉丝' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '刷新' })).toBeVisible()
  })

  test('深链账号不在首批候选时补取详情后允许同步', async ({ page }) => {
    await installDeepAccountRoutes(page)
    await page.goto(`/mp/fans?account_id=${secondAccount.id}`)

    await expect(page.locator('.list-filter-form')).toContainText(secondAccount.name)
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('尚未同步粉丝资料')
    await expect(page.getByRole('button', { name: '同步粉丝' })).toBeEnabled()
  })

  test('筛选草稿和新账号状态未确认时保持 fail-closed', async ({ page }) => {
    const state = await installAccountSwitchRoutes(page)
    await page.goto(`/mp/fans?account_id=${account.id}`)

    const syncButton = page.getByRole('button', { name: '同步粉丝' })
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('尚未同步粉丝资料')
    await expect(syncButton).toBeEnabled()

    const accountSelect = page.locator('.list-filter-form .ant-select')
    await accountSelect.click()
    await page
      .locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option')
      .filter({ hasText: secondAccount.name })
      .click()
    await expect(syncButton).toBeDisabled()
    expect(state.getSyncStarts()).toBe(0)

    await page.getByRole('button', { name: '查询' }).click()
    await state.secondStatusRequested
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('正在读取粉丝同步状态')
    await expect(syncButton).toBeDisabled()

    state.releaseSecondStatus()
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('尚未同步粉丝资料')
    await expect(syncButton).toBeEnabled()
    expect(state.getSyncStarts()).toBe(0)
  })

  test('失败任务可以重新启动并完成', async ({ page }) => {
    const state = await installFanRoutes(page, 'failed')
    await page.goto(`/mp/fans?account_id=${account.id}`)

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步失败')
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('模拟微信接口失败')
    const restartButton = page.getByRole('button', { name: '重新同步' })
    await expect(restartButton).toBeEnabled()
    await restartButton.click()

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('等待执行')
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步完成', { timeout: 8_000 })
    expect(state.getSyncStarts()).toBe(1)
  })

  test('刷新页面后恢复活动任务并在终态停止轮询', async ({ page }) => {
    const state = await installFanRoutes(page, 'running')
    await page.goto(`/mp/fans?account_id=${account.id}`)
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步中')

    await page.reload()
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步中')
    state.releaseRunning()
    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步完成', { timeout: 6_000 })

    const terminalReads = state.getJobReads()
    await page.waitForTimeout(2_500)
    expect(state.getJobReads()).toBe(terminalReads)
  })
})

test.describe('公众号 M4-B 粉丝页移动端', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test('状态区不撑破页面且宽表只在表格内滚动', async ({ page }) => {
    await installFanRoutes(page, 'succeeded')
    await page.goto(`/mp/fans?account_id=${account.id}`)

    await expect(page.locator('.mp-fans-sync-alert')).toContainText('同步完成')
    await expect(page.getByText(fan.nickname)).toBeVisible()

    const dimensions = await page.evaluate(() => {
      const table = document.querySelector('.mp-fans-page .ant-table-content') as HTMLElement | null
      return {
        rootClientWidth: document.documentElement.clientWidth,
        rootScrollWidth: document.documentElement.scrollWidth,
        tableClientWidth: table?.clientWidth ?? 0,
        tableScrollWidth: table?.scrollWidth ?? 0,
      }
    })
    expect(dimensions.rootScrollWidth).toBeLessThanOrEqual(dimensions.rootClientWidth)
    expect(dimensions.tableClientWidth).toBeGreaterThan(0)
    expect(dimensions.tableScrollWidth).toBeGreaterThan(dimensions.tableClientWidth)
  })
})
