import { test, expect } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

test.describe('登录态刷新', () => {
  test('两个页面同时失效时只刷新一次且不跳转登录', async ({ page }) => {
    const context = page.context()
    const peer = await context.newPage()
    let refreshCalls = 0

    await context.route('**/api/v1/user/me', async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ code: 401, message: 'access token expired' }),
      })
    })

    await context.route('**/api/v1/refresh', async (route) => {
      refreshCalls += 1
      if (refreshCalls > 1) {
        await route.fulfill({
          status: 401,
          contentType: 'application/json',
          body: JSON.stringify({ code: 401, message: 'refresh token already used' }),
        })
        return
      }

      // 模拟后端 rotation：第一个请求完成前，后续请求也不能复用旧 token。
      await new Promise((resolve) => setTimeout(resolve, 250))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          message: 'ok',
          data: {
            access_token: 'access-fixture',
            refresh_token: 'refresh-fixture',
          },
        }),
      })
    })

    try {
      await Promise.all([page.goto('/system/user'), peer.goto('/system/user')])

      await expect
        .poll(() => refreshCalls, { timeout: 5_000 })
        .toBe(1)
      await expect
        .poll(() => page.evaluate(() => localStorage.getItem('refresh_token')), {
          timeout: 5_000,
        })
        .toBe('refresh-fixture')
      await expect(page).not.toHaveURL(/\/login/)
      await expect(peer).not.toHaveURL(/\/login/)
    } finally {
      await peer.close()
    }
  })
})
