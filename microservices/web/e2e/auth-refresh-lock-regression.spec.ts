import { test, expect, type BrowserContext, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

const AUTH_REFRESH_LOCK_KEY = 'go-admin-kit-auth-refresh-lock'
const AUTH_TOKEN_PAIR_KEY = 'auth_token_pair'
const USER_ME_PATH = '/api/v1/user/me'

const userMeSuccess = {
  id: 1,
  username: 'refresh-lock-admin',
  nickname: '刷新锁测试用户',
  roles: [{ id: 1, name: '超级管理员', code: 'super_admin' }],
  permissions: [],
}

test.use({ storageState: STORAGE_STATE })

test.beforeEach(async ({ page }) => {
  await page.context().addInitScript(() => {
    try {
      localStorage.removeItem('auth_token_pair')
      localStorage.setItem('access_token', 'fixture-access')
      localStorage.setItem('refresh_token', 'fixture-refresh')
    } catch {
      // The initial about:blank document may not have a storage origin yet.
    }
  })
})

type IndexedDBFailure = 'error' | 'abort' | 'blocked' | 'closed'

const unauthorized = async (route: Route) => {
  await route.fulfill({
    status: 401,
    contentType: 'application/json',
    body: JSON.stringify({ code: 401, message: 'access token expired' }),
  })
}

function waitForUserMeSuccess(page: Page, accessToken: string) {
  return page.waitForResponse(
    (response) => (
      new URL(response.url()).pathname === USER_ME_PATH &&
      response.status() === 200 &&
      response.request().headers().authorization === `Bearer ${accessToken}`
    ),
    { timeout: 8_000 },
  )
}

async function assertUserMeSuccess(response: Awaited<ReturnType<typeof waitForUserMeSuccess>>, accessToken: string) {
  expect(response.status()).toBe(200)
  expect(response.request().headers().authorization).toBe(`Bearer ${accessToken}`)
  await expect(response.json()).resolves.toMatchObject({
    code: 200,
    data: { username: userMeSuccess.username },
  })
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

async function installIndexedDBFailure(context: BrowserContext, failure: IndexedDBFailure) {
  await context.addInitScript((mode: IndexedDBFailure) => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: undefined,
    })

    const createOpenRequest = () => {
      const request = {
        result: undefined as unknown,
        onupgradeneeded: null as ((event: Event) => void) | null,
        onsuccess: null as ((event: Event) => void) | null,
        onerror: null as ((event: Event) => void) | null,
        onblocked: null as ((event: Event) => void) | null,
      }
      window.setTimeout(() => {
        if (mode === 'blocked') request.onblocked?.(new Event('blocked'))
        else if (mode === 'error') request.onerror?.(new Event('error'))
      }, 0)
      return request
    }

    const state = window as Window & { indexedDBTransactionCalls?: number }
    state.indexedDBTransactionCalls = 0
    const database = {
      objectStoreNames: { contains: () => true },
      close: () => undefined,
      transaction: () => {
        state.indexedDBTransactionCalls = (state.indexedDBTransactionCalls || 0) + 1
        if (mode === 'closed') {
          throw new DOMException('The database connection is closing.', 'InvalidStateError')
        }

        const transaction = {
          oncomplete: null as (() => void) | null,
          onerror: null as (() => void) | null,
          onabort: null as (() => void) | null,
          objectStore: () => ({
            get: () => {
              const request = {
                result: undefined as unknown,
                onsuccess: null as ((event: Event) => void) | null,
              }
              window.setTimeout(() => request.onsuccess?.(new Event('success')), 0)
              return request
            },
            put: () => undefined,
            delete: () => undefined,
          }),
        }

        if (mode === 'abort') {
          window.setTimeout(() => transaction.onabort?.(), 0)
        }
        return transaction
      },
    }

    Object.defineProperty(window, 'indexedDB', {
      configurable: true,
      value: {
        open: () => {
          if (mode !== 'abort' && mode !== 'closed') return createOpenRequest()
          const request = createOpenRequest()
          request.result = database
          window.setTimeout(() => request.onsuccess?.(new Event('success')), 0)
          return request
        },
      },
    })
  }, failure)
}

async function installTransientIndexedDBFailure(context: BrowserContext) {
  await context.addInitScript(() => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: undefined,
    })

    const nativeIndexedDB = window.indexedDB
    let failNextOpen = true
    Object.defineProperty(window, 'indexedDB', {
      configurable: true,
      value: {
        open: (name: string, version?: number) => {
          if (failNextOpen) {
            failNextOpen = false
            throw new DOMException('IndexedDB is temporarily unavailable', 'InvalidStateError')
          }
          return nativeIndexedDB.open(name, version)
        },
      },
    })
    const controls = window as Window & { allowRefreshIndexedDB?: () => void }
    controls.allowRefreshIndexedDB = () => {
      failNextOpen = false
    }
  })
}

async function installRejectingWebLocks(context: BrowserContext) {
  await context.addInitScript(() => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: () => Promise.reject(new Error('Web Locks API rejected')),
      },
    })
  })
}

async function installImmediateWebLockNearDeadline(context: BrowserContext) {
  await context.addInitScript(() => {
    const realNow = Date.now
    let elapsed = 0
    Object.defineProperty(Date, 'now', {
      configurable: true,
      value: () => realNow() + elapsed,
    })
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: async (_name: string, _options: unknown, callback: (lock: object) => Promise<unknown>) => {
          elapsed = 19_700
          return callback({})
        },
      },
    })
  })
}

test.describe('authentication refresh lock regressions', () => {
  for (const failure of ['error', 'abort', 'blocked', 'closed'] as const) {
    test(`does not fall back to localStorage after IndexedDB ${failure}`, async ({ page }) => {
      await installIndexedDBFailure(page.context(), failure)
      let refreshCalls = 0

      await page.route('**/api/v1/user/me', unauthorized)
      await page.route('**/api/v1/refresh', async (route) => {
        refreshCalls += 1
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: { access_token: 'test-access-placeholder', refresh_token: 'test-refresh-placeholder' },
          }),
        })
      })

      await page.goto('/system/user')
      await page.waitForTimeout(1_200)

      expect(refreshCalls).toBe(0)
      await expect(page.evaluate((key) => localStorage.getItem(key), AUTH_REFRESH_LOCK_KEY)).resolves.toBeNull()
      if (failure === 'abort' || failure === 'closed') {
        await expect(page.evaluate(() => (window as Window & { indexedDBTransactionCalls?: number }).indexedDBTransactionCalls))
          .resolves.toBeGreaterThan(0)
      }
    })
  }

  test('falls back to IndexedDB coordination when Web Locks API rejects', async ({ page }) => {
    const context = page.context()
    await installRejectingWebLocks(context)
    const peer = await context.newPage()
    let refreshCalls = 0

    await context.route('**/api/v1/user/me', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-locks-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }
      await unauthorized(route)
    })
    await context.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      if (refreshCalls > 1) {
        await unauthorized(route)
        return
      }
      await new Promise((resolve) => setTimeout(resolve, 250))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { access_token: 'test-locks-access', refresh_token: 'test-locks-refresh' },
        }),
      })
    })

    try {
      const pageSuccess = waitForUserMeSuccess(page, 'test-locks-access')
      const peerSuccess = waitForUserMeSuccess(peer, 'test-locks-access')
      await Promise.all([page.goto('/system/user'), peer.goto('/system/user')])
      await assertUserMeSuccess(await pageSuccess, 'test-locks-access')
      await assertUserMeSuccess(await peerSuccess, 'test-locks-access')
      await expect.poll(() => refreshCalls, { timeout: 5_000 }).toBe(1)
      await expect
        .poll(() => page.evaluate(() => localStorage.getItem('refresh_token')), { timeout: 5_000 })
        .toBe('test-locks-refresh')
      await expect(page).not.toHaveURL(/\/login/)
      await expect(peer).not.toHaveURL(/\/login/)
      await expect
        .poll(() => page.evaluate((pairKey) => localStorage.getItem(pairKey), AUTH_TOKEN_PAIR_KEY))
        .toBe(JSON.stringify({ access: 'test-locks-access', refresh: 'test-locks-refresh' }))
    } finally {
      await peer.close()
    }
  })

  test('retries IndexedDB after a transient open failure without using localStorage', async ({ page }) => {
    await installTransientIndexedDBFailure(page.context())
    let refreshCalls = 0

    await page.route('**/api/v1/user/me', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-transient-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }
      await unauthorized(route)
    })
    await page.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          message: 'ok',
          data: { access_token: 'test-transient-access', refresh_token: 'test-transient-refresh' },
        }),
      })
    })

    const success = waitForUserMeSuccess(page, 'test-transient-access')
    await page.goto('/system/user', { waitUntil: 'domcontentloaded' })
    await page.evaluate(() => {
      const controls = window as Window & { allowRefreshIndexedDB?: () => void }
      controls.allowRefreshIndexedDB?.()
    })

    await assertUserMeSuccess(await success, 'test-transient-access')
    expect(refreshCalls).toBe(1)
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('new reader migrates a token pair written by an older page', async ({ page }) => {
    await page.context().addInitScript(() => {
      localStorage.setItem('auth_token_pair', JSON.stringify({ access: 'test-stale-access', refresh: 'test-stale-refresh' }))
      localStorage.setItem('access_token', 'test-legacy-access')
      localStorage.setItem('refresh_token', 'test-legacy-refresh')
    })
    await page.route('**/api/v1/user/me', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-legacy-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }
      await unauthorized(route)
    })
    await page.route('**/api/v1/user/menus', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'ok', data: [] }),
      })
    })

    const success = waitForUserMeSuccess(page, 'test-legacy-access')
    await page.goto('/system/user')
    await assertUserMeSuccess(await success, 'test-legacy-access')
    await expectTokenPair(page, 'test-legacy-access', 'test-legacy-refresh')
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('new reader accepts an older page login after the pair was cleared', async ({ page }) => {
    await page.context().addInitScript(() => {
      localStorage.setItem('auth_token_pair', JSON.stringify({ access: '', refresh: '' }))
      localStorage.setItem('access_token', 'test-legacy-login-access')
      localStorage.setItem('refresh_token', 'test-legacy-login-refresh')
    })
    await page.route('**/api/v1/user/me', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-legacy-login-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }
      await unauthorized(route)
    })
    await page.route('**/api/v1/user/menus', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'ok', data: [] }),
      })
    })

    const success = waitForUserMeSuccess(page, 'test-legacy-login-access')
    await page.goto('/system/user')
    await assertUserMeSuccess(await success, 'test-legacy-login-access')
    await expectTokenPair(page, 'test-legacy-login-access', 'test-legacy-login-refresh')
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('does not treat a refresh callback failure as a Web Locks API failure', async ({ page }) => {
    const context = page.context()
    await context.addInitScript(() => {
      Object.defineProperty(navigator, 'locks', {
        configurable: true,
        value: {
          request: async (_name: string, _options: unknown, callback: (lock: object) => Promise<unknown>) =>
            callback({}),
        },
      })
    })
    let refreshCalls = 0

    await page.route('**/api/v1/user/me', unauthorized)
    await page.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      if (refreshCalls === 1) {
        await unauthorized(route)
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { access_token: 'test-fallback-access', refresh_token: 'test-fallback-refresh' },
        }),
      })
    })

    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 })
    await page.waitForTimeout(300)
    expect(refreshCalls).toBe(1)
  })

  test('bounds an acquired lock refresh request by the shared 20 second deadline', async ({ page }) => {
    await installImmediateWebLockNearDeadline(page.context())
    await page.route('**/api/v1/user/me', unauthorized)
    await page.route('**/api/v1/refresh', async () => {
      await new Promise((resolve) => setTimeout(resolve, 60_000))
    })

    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/, { timeout: 3_000 })
  })

  test('bounds a never-settling Web Locks request by the shared 20 second deadline', async ({ page }) => {
    await page.context().addInitScript(() => {
      const realNow = Date.now
      let elapsed = 0
      Object.defineProperty(Date, 'now', {
        configurable: true,
        value: () => realNow() + elapsed,
      })
      Object.defineProperty(navigator, 'locks', {
        configurable: true,
        value: {
          request: () => {
            elapsed = 19_700
            return new Promise<never>(() => {})
          },
        },
      })
    })
    await page.route('**/api/v1/user/me', unauthorized)

    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/, { timeout: 3_000 })
  })

  test('bounds a never-settling IndexedDB open by the shared 20 second deadline', async ({ page }) => {
    await page.context().addInitScript(() => {
      const realNow = Date.now
      let elapsed = 0
      Object.defineProperty(Date, 'now', {
        configurable: true,
        value: () => realNow() + elapsed,
      })
      Object.defineProperty(navigator, 'locks', {
        configurable: true,
        value: undefined,
      })
      Object.defineProperty(window, 'indexedDB', {
        configurable: true,
        value: {
          open: () => {
            elapsed = 19_700
            return {}
          },
        },
      })
    })
    await page.route('**/api/v1/user/me', unauthorized)

    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/, { timeout: 3_000 })
  })

  test('bounds a never-settling IndexedDB transaction by the shared 20 second deadline', async ({ page }) => {
    await page.context().addInitScript(() => {
      const realNow = Date.now
      let elapsed = 0
      Object.defineProperty(Date, 'now', {
        configurable: true,
        value: () => realNow() + elapsed,
      })
      Object.defineProperty(navigator, 'locks', {
        configurable: true,
        value: undefined,
      })
      const database = {
        objectStoreNames: { contains: () => true },
        close: () => undefined,
        transaction: () => {
          elapsed = 19_700
          return {
            oncomplete: null as (() => void) | null,
            onerror: null as (() => void) | null,
            onabort: null as (() => void) | null,
            objectStore: () => ({
              get: () => ({
                result: undefined,
                onsuccess: null as ((event: Event) => void) | null,
                onerror: null as ((event: Event) => void) | null,
              }),
              put: () => undefined,
              delete: () => undefined,
            }),
          }
        },
      }
      Object.defineProperty(window, 'indexedDB', {
        configurable: true,
        value: {
          open: () => {
            const request = {
              result: database,
              onupgradeneeded: null as ((event: Event) => void) | null,
              onsuccess: null as ((event: Event) => void) | null,
              onerror: null as ((event: Event) => void) | null,
              onblocked: null as ((event: Event) => void) | null,
            }
            window.setTimeout(() => request.onsuccess?.(new Event('success')), 0)
            return request
          },
        },
      })
    })
    await page.route('**/api/v1/user/me', unauthorized)

    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/, { timeout: 3_000 })
  })

  test('bounds 401 recovery waiting by the same deadline', async ({ page }) => {
    await installImmediateWebLockNearDeadline(page.context())
    let refreshCalls = 0

    await page.route('**/api/v1/user/me', unauthorized)
    await page.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      await unauthorized(route)
    })

    await page.goto('/system/user')
    await expect.poll(() => refreshCalls, { timeout: 2_000 }).toBe(1)
    await expect(page).toHaveURL(/\/login/, { timeout: 1_500 })
  })

  test('reuses one refresh promise and one token pair for same-page 401s', async ({ page }) => {
    let refreshCalls = 0
    await page.route('**/api/v1/user/me', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-same-page-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: userMeSuccess }),
        })
        return
      }
      await unauthorized(route)
    })
    await page.route('**/api/v1/user/menus', async (route) => {
      if (route.request().headers().authorization === 'Bearer test-same-page-access') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, message: 'ok', data: [] }),
        })
        return
      }
      await unauthorized(route)
    })
    await page.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      await new Promise((resolve) => setTimeout(resolve, 250))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { access_token: 'test-same-page-access', refresh_token: 'test-same-page-refresh' },
        }),
      })
    })

    const userMeSuccessResponse = waitForUserMeSuccess(page, 'test-same-page-access')
    const menusSuccessResponse = page.waitForResponse(
      (response) => (
        new URL(response.url()).pathname === '/api/v1/user/menus' &&
        response.status() === 200 &&
        response.request().headers().authorization === 'Bearer test-same-page-access'
      ),
      { timeout: 8_000 },
    )
    await page.goto('/system/user')
    await assertUserMeSuccess(await userMeSuccessResponse, 'test-same-page-access')
    const menusResponse = await menusSuccessResponse
    expect(menusResponse.status()).toBe(200)
    expect(menusResponse.request().headers().authorization).toBe('Bearer test-same-page-access')
    await expect(menusResponse.json()).resolves.toMatchObject({ code: 200, data: [] })
    await expect.poll(() => refreshCalls, { timeout: 5_000 }).toBe(1)
    await expectTokenPair(page, 'test-same-page-access', 'test-same-page-refresh')
  })
})
