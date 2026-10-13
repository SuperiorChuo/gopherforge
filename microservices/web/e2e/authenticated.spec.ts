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

  test('Hermes 三分区与新增操作面可正常加载', async ({ page }) => {
    const consoleErrors: string[] = []
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text())
    })

    const pages = [
      { path: '/hermes/tasks', group: '协同运维', title: '互联任务' },
      { path: '/hermes/ai-ops', group: '协同运维', title: 'AI 运维' },
      { path: '/hermes/runtimes', group: '协同运维', title: '运行实例' },
      { path: '/hermes/skills', group: '资源中心', title: '技能市场' },
    ]

    for (const target of pages) {
      await page.goto(target.path)
      await expect(page).not.toHaveURL(/\/login/)
      await expect(page.getByRole('menuitem', { name: target.group })).toBeVisible()
      await expect(page.locator('.table-toolbar-heading')).toContainText(target.title, { timeout: 15_000 })
    }

    expect(consoleErrors).toEqual([])
  })

  test('客户查重失败显示错误态并支持重试', async ({ page }) => {
    let attempts = 0
    await page.route('**/api/v1/crm/dup-groups*', async (route) => {
      attempts += 1
      if (attempts === 1) {
        await route.abort('failed')
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { groups: [], total: 0 } }),
      })
    })

    await page.goto('/crm/customers')
    await page.getByRole('button', { name: '查重合并' }).click()
    await expect(page.getByText('无法加载重复客户，请重试。')).toBeVisible()
    await expect(page.getByText('未发现重复客户（按号码尾 11 位与同名检测）')).not.toBeVisible()

    await page.getByRole('button', { name: '重试' }).click()
    await expect(page.getByText('未发现重复客户（按号码尾 11 位与同名检测）')).toBeVisible()
  })

  test('登录日志地域分布地图渲染真实数据', async ({ page }) => {
    const geoResponsePromise = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/login-logs/geo') &&
        response.request().method() === 'GET',
    )

    await page.goto('/system/login-log')

    const geoResponse = await geoResponsePromise
    expect(geoResponse.ok()).toBeTruthy()
    const payload = (await geoResponse.json()) as {
      code: number
      data: Array<{ province?: string; city?: string }>
    }
    expect(payload.code).toBe(200)
    expect(payload.data.length).toBeGreaterThan(0)

    await expect(page.getByText('登录地域分布').first()).toBeVisible()
    const map = page.locator('svg[aria-label="登录地域分布图"]')
    await expect(map).toBeVisible()
    await expect(map.locator('.geo-country').first()).toBeVisible()

    // CI 可能只有内网登录记录，没有可落图坐标；有可定位数据时必须出现点位。
    const hasLocatableData = payload.data.some(
      ({ province, city }) =>
        Boolean(province && city) && !['内网', '未知', 'Private Network'].includes(province!),
    )
    if (hasLocatableData) {
      await expect(map.locator('.geo-point').first()).toBeVisible()
    }
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
