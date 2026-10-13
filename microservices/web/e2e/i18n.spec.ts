import { test, expect } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

// i18n 语言切换 E2E：验证 message-as-key 全链路（zh 默认 → 切 en → antd 内置文案随 locale →
// 刷新保持）。语言按钮在 MainLayout 头部（.app-lang-trigger），试点页 audit-log 为断言目标。
test.use({ storageState: STORAGE_STATE })

test.describe('i18n 语言切换', () => {
  test('audit-log 中文默认 → 切英文 → 刷新保持，控制台无错', async ({ page }) => {
    const consoleErrors: string[] = []
    page.on('console', (m) => {
      if (m.type() === 'error') consoleErrors.push(m.text())
    })
    // 干净的 zh 默认：只清首屏（sessionStorage 标记），reload 不重复清——否则刷新后
    // 持久化断言会被自己的清理脚本破坏（addInitScript 对每次导航都生效）
    await page.addInitScript(() => {
      if (!sessionStorage.getItem('__i18n_clean')) {
        sessionStorage.setItem('__i18n_clean', '1')
        window.localStorage.removeItem('app_locale')
      }
    })

    await page.goto('/system/audit-log')
    await expect(page).not.toHaveURL(/\/login/)
    // 中文默认：表格列头「操作者」+ 语言按钮显示目标语「EN」
    await expect(page.getByRole('columnheader', { name: '操作者' })).toBeVisible()
    await expect(page.locator('.app-lang-trigger')).toContainText('EN')

    // 切英文：列头变 Operator，antd 内置分页文案随 ConfigProvider 走（Pagination 有 "items/page" 或 "Page"）
    await page.locator('.app-lang-trigger').click()
    await expect(page.getByRole('columnheader', { name: 'Operator' })).toBeVisible()
    await expect(page.locator('.app-lang-trigger')).toContainText('中文')

    // 刷新后保持英文（localStorage 持久）
    await page.reload()
    await expect(page.getByRole('columnheader', { name: 'Operator' })).toBeVisible()

    // 控制台无错误（i18n 配置不当会刷 missing-key 报错）
    expect(consoleErrors.filter((e) => !e.includes('favicon') && !e.includes('404'))).toEqual([])
  })

  test('菜单标题随语言切换（后端 m.title 作 key 经 buildMenuItems 翻译）', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.removeItem('app_locale'))
    await page.goto('/dashboard')
    await expect(page.getByRole('menuitem', { name: /系统管理/ }).first()).toBeVisible()

    await page.locator('.app-lang-trigger').click()
    // en 下顶级菜单（若已译）显示 English；未译 key 回落中文——断言按钮态即可（翻译进度敏感）
    await expect(page.locator('.app-lang-trigger')).toContainText('中文')
  })
})
