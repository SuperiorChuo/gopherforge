import { test, expect, type BrowserContext, type Page } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const OLD_ACCESS_TOKEN = 'test-old-access'
const OLD_REFRESH_TOKEN = 'test-old-refresh'
const NEW_ACCESS_TOKEN = 'test-new-access'
const NEW_REFRESH_TOKEN = 'test-new-refresh'
const AUTH_TOKEN_PAIR_KEY = 'auth_token_pair'
const USER_ME_PATH = '/api/v1/user/me'
const REFRESH_PATH = '/api/v1/refresh'

type UserMeAttempt = {
  authorization: string | undefined
  status: number
}

type RefreshFixture = {
  userMeAttempts: UserMeAttempt[]
  refreshCalls: number
}

const userMeSuccess = {
  id: 1,
  username: 'refresh-admin',
  nickname: '刷新测试用户',
  roles: [{ id: 1, name: '超级管理员', code: 'super_admin' }],
  permissions: [],
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

async function installRefreshFixture(
  context: BrowserContext,
  options: { disableWebLocks?: boolean; refreshBehavior?: 'rotate' | 'timeout' } = {},
): Promise<RefreshFixture> {
  if (options.disableWebLocks) {
    await context.addInitScript(() => {
      Object.defineProperty(navigator, 'locks', {
        configurable: true,
        value: undefined,
      })
    })
  }

  await context.addInitScript(({ access, refresh }) => {
    localStorage.removeItem('auth_token_pair')
    localStorage.setItem('access_token', access)
    localStorage.setItem('refresh_token', refresh)
  }, { access: OLD_ACCESS_TOKEN, refresh: OLD_REFRESH_TOKEN })

  const fixture: RefreshFixture = {
    userMeAttempts: [],
    refreshCalls: 0,
  }

  await context.route('**/api/v1/**', async (route) => {
    const pathname = new URL(route.request().url()).pathname

    if (pathname === USER_ME_PATH) {
      const authorization = route.request().headers().authorization
      if (authorization === `Bearer ${OLD_ACCESS_TOKEN}`) {
        fixture.userMeAttempts.push({ authorization, status: 401 })
        await route.fulfill({
          status: 401,
          contentType: 'application/json',
          body: JSON.stringify({ code: 401, message: 'access token expired' }),
        })
        return
      }

      if (authorization === `Bearer ${NEW_ACCESS_TOKEN}`) {
        fixture.userMeAttempts.push({ authorization, status: 200 })
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }

      fixture.userMeAttempts.push({ authorization, status: 401 })
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ code: 401, message: 'unexpected access token' }),
      })
      return
    }

    if (pathname === REFRESH_PATH) {
      fixture.refreshCalls += 1
      if (options.refreshBehavior === 'timeout') {
        // Leave the request pending long enough for Axios' 15 second timeout
        // to fire; this exercises the client timeout path rather than a
        // synthetic route abort.
        await wait(16_000)
        await route.abort('timedout')
        return
      }

      if (fixture.refreshCalls > 1) {
        await route.fulfill({
          status: 401,
          contentType: 'application/json',
          body: JSON.stringify({ code: 401, message: 'refresh token already used' }),
        })
        return
      }

      await wait(250)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          message: 'ok',
          data: {
            access_token: NEW_ACCESS_TOKEN,
            refresh_token: NEW_REFRESH_TOKEN,
          },
        }),
      })
      return
    }

    const data = pathname === '/api/v1/user/menus'
      ? []
      : { list: [], total: 0, page: 1, page_size: 10 }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, message: 'ok', data }),
    })
  })

  return fixture
}

function waitForUserMeResponse(page: Page, status: number) {
  return page.waitForResponse(
    (response) => new URL(response.url()).pathname === USER_ME_PATH && response.status() === status,
    { timeout: 8_000 },
  )
}

async function assertUserMeResponse(
  response: {
    status(): number
    request(): { headers(): Record<string, string> }
    json(): Promise<unknown>
  },
  expectedStatus: number,
  expectedAuthorization: string,
) {
  expect(response.status()).toBe(expectedStatus)
  expect(response.request().headers().authorization).toBe(expectedAuthorization)
  if (expectedStatus === 200) {
    await expect(response.json()).resolves.toMatchObject({
      code: 200,
      data: { username: userMeSuccess.username },
    })
  }
}

function assertRotatedUserMeAttempts(fixture: RefreshFixture, expectedSuccesses: number) {
  const oldTokenFailures = fixture.userMeAttempts.filter((attempt) => (
    attempt.authorization === `Bearer ${OLD_ACCESS_TOKEN}` && attempt.status === 401
  ))
  const newTokenSuccesses = fixture.userMeAttempts.filter((attempt) => (
    attempt.authorization === `Bearer ${NEW_ACCESS_TOKEN}` && attempt.status === 200
  ))

  expect(oldTokenFailures.length).toBeGreaterThanOrEqual(expectedSuccesses)
  expect(newTokenSuccesses.length).toBe(oldTokenFailures.length)
  expect(fixture.userMeAttempts).toHaveLength(oldTokenFailures.length + newTokenSuccesses.length)
}

function expectTokenPair(page: Page, access: string, refresh: string) {
  return expect
    .poll(() => page.evaluate((pairKey) => {
      let pair: unknown = null
      try {
        pair = JSON.parse(localStorage.getItem(pairKey) || 'null')
      } catch {
        pair = null
      }
      return {
        access: localStorage.getItem('access_token'),
        refresh: localStorage.getItem('refresh_token'),
        pair,
      }
    }, AUTH_TOKEN_PAIR_KEY))
    .toEqual({ access, refresh, pair: { access, refresh } })
}

test.describe('登录态刷新', () => {
  test('两个页面同时失效时只刷新一次，并用新 token 重试 user/me 成功', async ({ page }) => {
    const context = page.context()
    const fixture = await installRefreshFixture(context)
    const peer = await context.newPage()

    try {
      const pageExpiredUserMe = waitForUserMeResponse(page, 401)
      const peerExpiredUserMe = waitForUserMeResponse(peer, 401)
      const pageSuccessfulUserMe = waitForUserMeResponse(page, 200)
      const peerSuccessfulUserMe = waitForUserMeResponse(peer, 200)
      await Promise.all([page.goto('/403'), peer.goto('/403')])

      const [pageExpiredResponse, peerExpiredResponse, pageResponse, peerResponse] = await Promise.all([
        pageExpiredUserMe,
        peerExpiredUserMe,
        pageSuccessfulUserMe,
        peerSuccessfulUserMe,
      ])
      await assertUserMeResponse(pageExpiredResponse, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
      await assertUserMeResponse(peerExpiredResponse, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
      await assertUserMeResponse(pageResponse, 200, `Bearer ${NEW_ACCESS_TOKEN}`)
      await assertUserMeResponse(peerResponse, 200, `Bearer ${NEW_ACCESS_TOKEN}`)

      expect(fixture.refreshCalls).toBe(1)
      assertRotatedUserMeAttempts(fixture, 1)
      await expectTokenPair(page, NEW_ACCESS_TOKEN, NEW_REFRESH_TOKEN)
      await expectTokenPair(peer, NEW_ACCESS_TOKEN, NEW_REFRESH_TOKEN)
    } finally {
      await peer.close()
    }
  })

  test('无 Web Locks 时使用 IndexedDB 租约协调刷新，并用新 token 重试 user/me 成功', async ({ page }) => {
    const context = page.context()
    const fixture = await installRefreshFixture(context, { disableWebLocks: true })
    const peer = await context.newPage()

    try {
      const pageExpiredUserMe = waitForUserMeResponse(page, 401)
      const peerExpiredUserMe = waitForUserMeResponse(peer, 401)
      const pageSuccessfulUserMe = waitForUserMeResponse(page, 200)
      const peerSuccessfulUserMe = waitForUserMeResponse(peer, 200)
      await Promise.all([page.goto('/403'), peer.goto('/403')])

      const [pageExpiredResponse, peerExpiredResponse, pageResponse, peerResponse] = await Promise.all([
        pageExpiredUserMe,
        peerExpiredUserMe,
        pageSuccessfulUserMe,
        peerSuccessfulUserMe,
      ])
      await expect.poll(() => page.evaluate(() => navigator.locks === undefined)).toBe(true)
      await assertUserMeResponse(pageExpiredResponse, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
      await assertUserMeResponse(peerExpiredResponse, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
      await assertUserMeResponse(pageResponse, 200, `Bearer ${NEW_ACCESS_TOKEN}`)
      await assertUserMeResponse(peerResponse, 200, `Bearer ${NEW_ACCESS_TOKEN}`)

      expect(fixture.refreshCalls).toBe(1)
      assertRotatedUserMeAttempts(fixture, 1)
      await expect
        .poll(
          () =>
            page.evaluate(async () => {
              const databases = await indexedDB.databases()
              return databases.some((database) => database.name === 'go-admin-kit-auth')
            }),
          { timeout: 5_000 },
        )
        .toBe(true)
      await expectTokenPair(page, NEW_ACCESS_TOKEN, NEW_REFRESH_TOKEN)
      await expectTokenPair(peer, NEW_ACCESS_TOKEN, NEW_REFRESH_TOKEN)
    } finally {
      await peer.close()
    }
  })

  test('refresh 请求卡住时在有限时间内最终跳转登录，而不是无限等待', async ({ page }) => {
    const fixture = await installRefreshFixture(page.context(), { refreshBehavior: 'timeout' })
    const expiredUserMe = waitForUserMeResponse(page, 401)

    await page.goto('/403', { waitUntil: 'domcontentloaded' })
    await expect(page).toHaveURL(/\/login/, { timeout: 18_000 })
    await assertUserMeResponse(await expiredUserMe, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
    expect(fixture.refreshCalls).toBe(1)
    expect(fixture.userMeAttempts.length).toBeGreaterThanOrEqual(1)
    expect(fixture.userMeAttempts.every((attempt) => (
      attempt.authorization === `Bearer ${OLD_ACCESS_TOKEN}` && attempt.status === 401
    ))).toBe(true)
  })

  test('自动刷新后退出登录使用最新 refresh token', async ({ page }) => {
    const fixture = await installRefreshFixture(page.context())
    let logoutBody: unknown = null

    await page.route('**/api/v1/logout', async (route) => {
      logoutBody = route.request().postDataJSON()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'logout success', data: null }),
      })
    })

    const expiredUserMe = waitForUserMeResponse(page, 401)
    const successfulUserMe = waitForUserMeResponse(page, 200)
    await page.goto('/403')
    await assertUserMeResponse(await expiredUserMe, 401, `Bearer ${OLD_ACCESS_TOKEN}`)
    await assertUserMeResponse(await successfulUserMe, 200, `Bearer ${NEW_ACCESS_TOKEN}`)

    await page.locator('.app-user').click()
    await page.getByRole('menuitem', { name: '退出登录' }).click()
    await expect(page).toHaveURL(/\/login/)
    expect(logoutBody).toEqual({ refresh_token: NEW_REFRESH_TOKEN })
    expect(fixture.refreshCalls).toBe(1)
  })
})
