import { test, expect } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

// 已登录态：复用 global-setup 写好的 token。验证认证后应用能正常工作——菜单渲染、
// 关键业务页可导航加载。这是 E2E 的主战场（登录本身由 API 冒烟覆盖）。
test.use({ storageState: STORAGE_STATE })

test.describe('认证后应用', () => {
  test('登录态进入后渲染主框架与侧边菜单', async ({ page }) => {
    await page.goto('/dashboard')
    // 未被踢回登录
    await expect(page).not.toHaveURL(/\/login/)
    // 侧边栏至少出现「系统管理」菜单（种子必有）
    await expect(page.getByRole('menuitem', { name: /系统管理/ }).first()).toBeVisible()
  })

  // 关键页导航冒烟：直接访问路由，断言不报错、不掉登录、页面出内容。
  const pages: Array<{ path: string; expect: RegExp }> = [
    { path: '/system/user', expect: /用户管理|用户/ },
    { path: '/system/menu', expect: /菜单/ },
    { path: '/monitor/server', expect: /服务器|监控/ },
    { path: '/social/content', expect: /内容中心|内容/ },
  ]

  for (const p of pages) {
    test(`访问 ${p.path} 正常加载`, async ({ page }) => {
      await page.goto(p.path)
      await expect(page).not.toHaveURL(/\/login/)
      // 页面主体出现预期文案（标题/工具栏），且没有整页崩溃
      await expect(page.getByText(p.expect).first()).toBeVisible({ timeout: 15_000 })
    })
  }
})
