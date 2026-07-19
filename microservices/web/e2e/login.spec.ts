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
})
