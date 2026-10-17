import { test, expect } from '@playwright/test'

// 登录页 E2E：不走完整验证码登录（像素解码在浏览器里不稳，登录逻辑由 API 冒烟覆盖），
// 只验证登录页能正常渲染出关键表单元素——防"登录页整体崩了"这类回归。
test.describe('登录页', () => {
  // 该 describe 用未登录态。
  test.use({ storageState: { cookies: [], origins: [] } })

  test('渲染用户名/密码/验证码表单', async ({ page }) => {
    await page.goto('/login')
    await expect(page.getByPlaceholder('用户名')).toBeVisible()
    await expect(page.getByPlaceholder('密码')).toBeVisible()
    // 登录按钮存在
    await expect(page.getByRole('button', { name: /登\s*录|登录/ })).toBeVisible()
  })

  test('未登录访问受保护页跳登录', async ({ page }) => {
    await page.goto('/system/user')
    await expect(page).toHaveURL(/\/login/)
  })

  test('主题切换只捕获旧画面并从按钮位置揭幕', async ({ page }) => {
    await page.addInitScript(() => {
      const probe = {
        calls: 0,
        oldName: '',
        newName: '',
        x: 0,
        y: 0,
      }
      Object.assign(window, { __viewTransitionProbe: probe })
      Object.defineProperty(document, 'startViewTransition', {
        configurable: true,
        value: (update: () => void) => {
          const root = document.documentElement
          probe.calls += 1
          probe.oldName = root.style.viewTransitionName
          probe.x = Number.parseFloat(root.style.getPropertyValue('--theme-reveal-x'))
          probe.y = Number.parseFloat(root.style.getPropertyValue('--theme-reveal-y'))
          update()
          probe.newName = root.style.viewTransitionName
          return {
            ready: Promise.resolve(),
            finished: new Promise<void>(() => {}),
            skipTransition: () => {},
          }
        },
      })
    })
    await page.goto('/login')
    const root = page.locator('html')
    await expect(root).toHaveAttribute('data-theme', 'dark')

    await page.getByRole('button', { name: /^切换(?:亮色|深色)$/ }).click()

    await expect(root).toHaveAttribute('data-theme', 'light')
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as Window & {
              __viewTransitionProbe: {
                calls: number
                oldName: string
                newName: string
                x: number
                y: number
              }
            }).__viewTransitionProbe,
        ),
      )
      .toMatchObject({ calls: 1, oldName: 'theme-old', newName: 'none' })
    const probe = await page.evaluate(
      () =>
        (window as Window & {
          __viewTransitionProbe: { x: number; y: number }
        }).__viewTransitionProbe,
    )
    expect(probe.x).toBeGreaterThan(0)
    expect(probe.y).toBeGreaterThan(0)
  })

  test('主题动画未开始时快速连点以最后一次切换为准', async ({ page }) => {
    await page.addInitScript(() => {
      const pending = new Promise<void>(() => {})
      Object.assign(window, { __skippedThemeTransitions: 0 })
      Object.defineProperty(document, 'startViewTransition', {
        configurable: true,
        value: () => ({
          ready: pending,
          finished: pending,
          skipTransition: () => {
            ;(
              window as Window & { __skippedThemeTransitions: number }
            ).__skippedThemeTransitions += 1
          },
        }),
      })
    })
    await page.goto('/login')
    const root = page.locator('html')
    const toggle = page.getByRole('button', { name: /^切换(?:亮色|深色)$/ })
    await expect(root).toHaveAttribute('data-theme', 'dark')

    await toggle.click()
    await toggle.click()

    await expect(root).toHaveAttribute('data-theme', 'dark')
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as Window & { __skippedThemeTransitions: number })
              .__skippedThemeTransitions,
        ),
      )
      .toBe(1)
  })
})
